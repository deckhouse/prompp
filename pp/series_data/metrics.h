#pragma once

#include "common.h"
#include "metrics/storage.h"
#include "series_data/encoder/timestamp/encoder.h"

namespace series_data {

template <class Reallocator>
struct Metrics final : metrics::MetricsPage<Metrics<Reallocator>> {
  using metrics::MetricsPage<Metrics>::MetricsPage;

  PROMPP_ALWAYS_INLINE explicit Metrics(const encoder::timestamp::Encoder<Reallocator>& timestamp_encoder)
      : metrics::MetricsPage<Metrics>(outdated_samples_count_), timestamp_encoder_(&timestamp_encoder) {}

  void refresh_metrics() noexcept override { timestamp_states_count_.set(timestamp_encoder_->states_count()); }

  PROMPP_ALWAYS_INLINE void inc_chunk_count(EncodingType encoding_type) noexcept {
    get_chunk_count(encoding_type, [](metrics::Gauge& gauge) PROMPP_LAMBDA_INLINE { gauge.inc(); });
  }
  PROMPP_ALWAYS_INLINE void dec_chunk_count(EncodingType encoding_type) noexcept {
    get_chunk_count(encoding_type, [](metrics::Gauge& gauge) PROMPP_LAMBDA_INLINE { gauge.dec(); });
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE double get_chunk_count(EncodingType encoding_type) const noexcept {
    return get_chunk_count(encoding_type, [](const metrics::Gauge& gauge) PROMPP_LAMBDA_INLINE { return gauge.value(); });
  }
  PROMPP_ALWAYS_INLINE void change_chunk_count(EncodingType from, EncodingType to) noexcept {
    dec_chunk_count(from);
    inc_chunk_count(to);
  }

  PROMPP_ALWAYS_INLINE metrics::Counter& outdated_samples() noexcept { return outdated_samples_count_; }
  PROMPP_ALWAYS_INLINE metrics::Counter& outdated_chunks() noexcept { return outdated_chunks_count_; }
  PROMPP_ALWAYS_INLINE metrics::Gauge& finalized_chunks() noexcept { return finalized_chunks_count_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE double timestamp_states_count() const noexcept { return timestamp_states_count_.value(); }

 private:
  const encoder::timestamp::Encoder<Reallocator>* timestamp_encoder_;
  const std::string ptr_label_{std::to_string(std::bit_cast<uint64_t>(this))};
  const std::array<PromPP::Primitives::LabelView, 1> labels_{PromPP::Primitives::LabelView{"address", ptr_label_}};

  metrics::Counter outdated_samples_count_{labels_, "prompp_data_storage_outdated_samples_count"};
  metrics::Counter outdated_chunks_count_{labels_, "prompp_data_storage_outdated_chunks_count"};
  metrics::Gauge finalized_chunks_count_{labels_, "prompp_data_storage_finalized_chunks_count"};
  metrics::Gauge timestamp_states_count_{labels_, "prompp_data_storage_timestamp_states_count"};

  metrics::Gauge uint32_constants_count_{labels_, "prompp_data_storage_uint32_constants_count"};
  metrics::Gauge float32_constants_count_{labels_, "prompp_data_storage_float32_constants_count"};
  metrics::Gauge double_constants_count_{labels_, "prompp_data_storage_double_constants_count"};
  metrics::Gauge two_double_constants_count_{labels_, "prompp_data_storage_two_double_constants_count"};
  metrics::Gauge asc_int_count_{labels_, "prompp_data_storage_asc_int_count"};
  metrics::Gauge asc_int_then_values_gorilla_count_{labels_, "prompp_data_storage_asc_int_then_values_gorilla_count"};
  metrics::Gauge values_gorilla_count_{labels_, "prompp_data_storage_values_gorilla_count"};
  metrics::Gauge gorilla_count_{labels_, "prompp_data_storage_gorilla_count"};

  template <class Handler>
  PROMPP_ALWAYS_INLINE decltype(auto) get_chunk_count(EncodingType encoding_type, Handler&& handler) noexcept {
    return get_chunk_count_impl(*this, encoding_type, std::forward<Handler>(handler));
  }

  template <class Handler>
  PROMPP_ALWAYS_INLINE decltype(auto) get_chunk_count(EncodingType encoding_type, Handler&& handler) const noexcept {
    return get_chunk_count_impl(*this, encoding_type, std::forward<Handler>(handler));
  }

  template <class MetricsRef, class Handler>
  PROMPP_ALWAYS_INLINE static decltype(auto) get_chunk_count_impl(MetricsRef&& metrics, EncodingType encoding_type, Handler&& handler) noexcept {
    switch (encoding_type) {
      case EncodingType::kUint32Constant:
        return std::forward<Handler>(handler)(metrics.uint32_constants_count_);

      case EncodingType::kFloat32Constant:
        return std::forward<Handler>(handler)(metrics.float32_constants_count_);

      case EncodingType::kDoubleConstant:
        return std::forward<Handler>(handler)(metrics.double_constants_count_);

      case EncodingType::kTwoDoubleConstant:
        return std::forward<Handler>(handler)(metrics.two_double_constants_count_);

      case EncodingType::kAscInteger:
        return std::forward<Handler>(handler)(metrics.asc_int_count_);

      case EncodingType::kAscIntegerThenValuesGorilla:
        return std::forward<Handler>(handler)(metrics.asc_int_then_values_gorilla_count_);

      case EncodingType::kValuesGorilla:
        return std::forward<Handler>(handler)(metrics.values_gorilla_count_);

      case EncodingType::kGorilla:
        return std::forward<Handler>(handler)(metrics.gorilla_count_);

      default:
        assert(false && "Unknown encoding type");
    }
  }
};

}  // namespace series_data
