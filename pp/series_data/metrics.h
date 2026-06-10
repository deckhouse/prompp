#pragma once

#include "common.h"
#include "metrics/storage.h"

namespace series_data {

struct Metrics final : metrics::MetricsPage<Metrics> {
  using MetricsPage::MetricsPage;

  explicit Metrics(const double& timestamp_states_count)
      : MetricsPage(outdated_samples_count), timestamp_states_count{labels_, "prompp_data_storage_timestamp_states_count", &timestamp_states_count} {}

  PROMPP_ALWAYS_INLINE void inc_chunk_count(EncodingType encoding_type) noexcept { get_chunk_count(encoding_type).inc(); }
  PROMPP_ALWAYS_INLINE void dec_chunk_count(EncodingType encoding_type) noexcept { get_chunk_count(encoding_type).dec(); }
  PROMPP_ALWAYS_INLINE void change_chunk_count(EncodingType from, EncodingType to) noexcept {
    dec_chunk_count(from);
    inc_chunk_count(to);
  }

  PROMPP_ALWAYS_INLINE void inc_outdated_samples() noexcept { outdated_samples_count.inc(); }
  PROMPP_ALWAYS_INLINE void inc_outdated_chunks() noexcept { outdated_chunks_count.inc(); }
  PROMPP_ALWAYS_INLINE void inc_finalized_chunks() noexcept { finalized_chunks_count.inc(); }

 private:
  const std::string ptr_label_{std::to_string(std::bit_cast<uint64_t>(this))};
  const std::array<PromPP::Primitives::LabelView, 1> labels_{PromPP::Primitives::LabelView{"ptr", ptr_label_}};

  metrics::Counter outdated_samples_count{labels_, "prompp_data_storage_outdated_samples_count"};
  metrics::Counter outdated_chunks_count{labels_, "prompp_data_storage_outdated_chunks_count"};
  metrics::Counter finalized_chunks_count{labels_, "prompp_data_storage_finalized_chunks_count"};

  metrics::GaugeRef timestamp_states_count;

  metrics::Gauge uint32_constants_count{labels_, "prompp_data_storage_uint32_constants_count"};
  metrics::Gauge float32_constants_count{labels_, "prompp_data_storage_float32_constants_count"};
  metrics::Gauge double_constants_count{labels_, "prompp_data_storage_double_constants_count"};
  metrics::Gauge two_double_constants_count{labels_, "prompp_data_storage_double_two_constants_count"};
  metrics::Gauge asc_int_count{labels_, "prompp_data_storage_asc_int_count"};
  metrics::Gauge asc_int_then_values_gorilla_count{labels_, "prompp_data_storage_asc_int_then_values_gorilla_count"};
  metrics::Gauge values_gorilla_count{labels_, "prompp_data_storage_values_gorilla_count"};
  metrics::Gauge gorilla_count{labels_, "prompp_data_storage_gorilla_count"};

  [[nodiscard]] PROMPP_ALWAYS_INLINE metrics::Gauge& get_chunk_count(EncodingType encoding_type) noexcept {
    switch (encoding_type) {
      case EncodingType::kUint32Constant:
        return uint32_constants_count;

      case EncodingType::kFloat32Constant:
        return float32_constants_count;

      case EncodingType::kDoubleConstant:
        return double_constants_count;

      case EncodingType::kTwoDoubleConstant:
        return two_double_constants_count;

      case EncodingType::kAscInteger:
        return asc_int_count;

      case EncodingType::kAscIntegerThenValuesGorilla:
        return asc_int_then_values_gorilla_count;

      case EncodingType::kValuesGorilla:
        return values_gorilla_count;

      case EncodingType::kGorilla:
        return gorilla_count;

      default:
        assert(false && "Unknown encoding type");
        return uint32_constants_count;
    }
  }
};

}  // namespace series_data