#pragma once

#include "snappy.h"

#include "bare_bones/vector.h"
#include "metric.h"
#include "prometheus/hashdex.h"
#include "prometheus/remote_write.h"

namespace PromPP::WAL::hashdex {

class Protobuf : public Prometheus::hashdex::Abstract {
 public:
  Protobuf() noexcept = default;
  explicit PROMPP_ALWAYS_INLINE Protobuf(const Prometheus::hashdex::Limits& limits) noexcept
      : limits_{
            .max_label_name_length = limits.max_label_name_length,
            .max_label_value_length = limits.max_label_value_length,
            .max_label_names_per_timeseries = limits.max_label_names_per_timeseries,
            .max_timeseries_count = limits.max_timeseries_count,
        } {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE const auto& limits() const noexcept { return limits_; }

  void presharding(std::string_view protobuf) {
    enum Tag : uint8_t {
      kTimeseries = 1,
      kMetadata = 3,
    };

    protozero::pbf_reader pb(protobuf);

    try {
      while (pb.next()) {
        switch (pb.tag()) {
          case kTimeseries: {
            parse_timeseries(pb);
            break;
          }

          case kMetadata: {
            parse_metadata(pb.get_message());
            break;
          }

          default: {
            pb.skip();
          }
        }
      }
    } catch (protozero::exception& e) {
      throw BareBones::Exception(0xbe40bda82f01b869, "Protobuf parsing timeseries exception: %s", e.what());
    }
  };

  // snappy_presharding uncompress protobuf via snappy and make presharding slice with hash and proto.
  PROMPP_ALWAYS_INLINE void snappy_presharding(std::string_view snappy_protobuf) {
    snappy::Uncompress(snappy_protobuf.data(), snappy_protobuf.size(), &protobuf_);
    presharding(protobuf_);
  };

  [[nodiscard]] PROMPP_ALWAYS_INLINE const auto& floats() const noexcept { return floats_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const auto& histograms() const noexcept { return histograms_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const auto& metadata() const noexcept { return metadata_; }

 private:
  class FloatItem {
    size_t hash_;
    std::string_view data_;

   public:
    PROMPP_ALWAYS_INLINE explicit FloatItem(size_t hash, std::string_view data) noexcept : hash_(hash), data_(data) {}
    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t hash() const { return hash_; }

    template <class Timeseries>
    PROMPP_ALWAYS_INLINE void read(Timeseries& timeseries) const {
      Prometheus::RemoteWrite::read_timeseries(protozero::pbf_reader(data_), timeseries);
    }
  };

  class HistogramItem {
    size_t hash_;
    std::string_view data_;

   public:
    PROMPP_ALWAYS_INLINE explicit HistogramItem(size_t hash, std::string_view data) noexcept : hash_(hash), data_(data) {}
    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t hash() const { return hash_; }

    template <class HistogramTimeseries>
    PROMPP_ALWAYS_INLINE void read(HistogramTimeseries& histogram_timeseries) const {
      Prometheus::RemoteWrite::read_histogram_timeseries(protozero::pbf_reader(data_), histogram_timeseries);
    }
  };

  std::string protobuf_;
  BareBones::Vector<FloatItem> floats_;
  BareBones::Vector<HistogramItem> histograms_;
  BareBones::Vector<Metadata> metadata_;
  const Prometheus::RemoteWrite::PbLabelSetMemoryLimits limits_{};
  Primitives::LabelViewSet label_set_;

  void parse_timeseries(protozero::pbf_reader& pb) {
    if (limits_.max_timeseries_count_exceeded(floats_.size())) [[unlikely]] {
      throw BareBones::Exception(0xdedb5b24d946cc4d, "Max Timeseries count limit exceeded");
    }

    label_set_.clear();
    auto pb_view = pb.get_view();
    const auto metrics_type = preparse_timeseries(protozero::pbf_reader{pb_view}, label_set_, limits_);

    if (floats_.empty() && histograms_.empty()) [[unlikely]] {
      set_cluser_and_replica_values(label_set_);
    }

    const auto hash = hash_value(label_set_);
    if (metrics_type == Prometheus::RemoteWrite::MetricsType::kFloat) [[likely]] {
      floats_.emplace_back(hash, pb_view);
    } else if (metrics_type == Prometheus::RemoteWrite::MetricsType::kHistogram) {
      histograms_.emplace_back(hash, pb_view);
    } else {
      throw BareBones::Exception(0x4108761111389831, "No timeseries data");
    }
  }

  void parse_metadata(protozero::pbf_reader pb_metadata) {
    enum Tag : uint8_t {
      kType = 1,
      kMetricName = 2,
      kHelp = 4,
      kUnit = 5,
    };

    auto& metadata = metadata_.emplace_back();

    while (pb_metadata.next()) {
      switch (pb_metadata.tag()) {
        case kType: {
          metadata.type = Prometheus::MetadataType::kType;
          metadata.text = Prometheus::MetricTypeToString(pb_metadata.get_uint32());
          break;
        }

        case kHelp: {
          metadata.type = Prometheus::MetadataType::kHelp;
          metadata.text = pb_metadata.get_view();
          break;
        }

        case kUnit: {
          metadata.type = Prometheus::MetadataType::kUnit;
          metadata.text = pb_metadata.get_view();
          break;
        }

        case kMetricName: {
          metadata.metric_name = pb_metadata.get_view();
          break;
        }

        default: {
          pb_metadata.skip();
        }
      }
    };
  }
};

static_assert(Prometheus::hashdex::HashdexInterface<Protobuf>);

}  // namespace PromPP::WAL::hashdex
