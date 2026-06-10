#pragma once

#include "common.h"
#include "metrics/storage.h"

namespace series_data {

struct Metrics final : metrics::MetricsPage<Metrics> {
  using MetricsPage::MetricsPage;

  PROMPP_ALWAYS_INLINE explicit Metrics() : MetricsPage(outdated_samples_count) {}

  PROMPP_ALWAYS_INLINE void inc_chunk_count(EncodingType encoding_type) noexcept {
    get_chunk_count(encoding_type, [](metrics::Gauge& gauge) PROMPP_LAMBDA_INLINE { gauge.inc(); });
  }
  PROMPP_ALWAYS_INLINE void dec_chunk_count(EncodingType encoding_type) noexcept {
    get_chunk_count(encoding_type, [](metrics::Gauge& gauge) PROMPP_LAMBDA_INLINE { gauge.dec(); });
  }
  PROMPP_ALWAYS_INLINE void change_chunk_count(EncodingType from, EncodingType to) noexcept {
    dec_chunk_count(from);
    inc_chunk_count(to);
  }

  PROMPP_ALWAYS_INLINE void inc_outdated_samples() noexcept { outdated_samples_count.inc(); }
  PROMPP_ALWAYS_INLINE void inc_outdated_chunks() noexcept { outdated_chunks_count.inc(); }
  PROMPP_ALWAYS_INLINE metrics::Gauge& finalized_chunks() noexcept { return finalized_chunks_count; }

 private:
  const std::string ptr_label_{std::to_string(std::bit_cast<uint64_t>(this))};
  const std::array<PromPP::Primitives::LabelView, 1> labels_{PromPP::Primitives::LabelView{"address", ptr_label_}};

  metrics::Counter outdated_samples_count{labels_, "prompp_data_storage_outdated_samples_count"};
  metrics::Counter outdated_chunks_count{labels_, "prompp_data_storage_outdated_chunks_count"};
  metrics::Gauge finalized_chunks_count{labels_, "prompp_data_storage_finalized_chunks_count"};

  metrics::Gauge uint32_constants_count{labels_, "prompp_data_storage_uint32_constants_count"};
  metrics::Gauge float32_constants_count{labels_, "prompp_data_storage_float32_constants_count"};
  metrics::Gauge double_constants_count{labels_, "prompp_data_storage_double_constants_count"};
  metrics::Gauge two_double_constants_count{labels_, "prompp_data_storage_two_double_constants_count"};
  metrics::Gauge asc_int_count{labels_, "prompp_data_storage_asc_int_count"};
  metrics::Gauge asc_int_then_values_gorilla_count{labels_, "prompp_data_storage_asc_int_then_values_gorilla_count"};
  metrics::Gauge values_gorilla_count{labels_, "prompp_data_storage_values_gorilla_count"};
  metrics::Gauge gorilla_count{labels_, "prompp_data_storage_gorilla_count"};

  template <class Handler>
  PROMPP_ALWAYS_INLINE void get_chunk_count(EncodingType encoding_type, Handler&& handler) noexcept {
    switch (encoding_type) {
      case EncodingType::kUint32Constant:
        return std::forward<Handler>(handler)(uint32_constants_count);

      case EncodingType::kFloat32Constant:
        return std::forward<Handler>(handler)(float32_constants_count);

      case EncodingType::kDoubleConstant:
        return std::forward<Handler>(handler)(double_constants_count);

      case EncodingType::kTwoDoubleConstant:
        return std::forward<Handler>(handler)(two_double_constants_count);

      case EncodingType::kAscInteger:
        return std::forward<Handler>(handler)(asc_int_count);

      case EncodingType::kAscIntegerThenValuesGorilla:
        return std::forward<Handler>(handler)(asc_int_then_values_gorilla_count);

      case EncodingType::kValuesGorilla:
        return std::forward<Handler>(handler)(values_gorilla_count);

      case EncodingType::kGorilla:
        return std::forward<Handler>(handler)(gorilla_count);

      default:
        assert(false && "Unknown encoding type");
    }
  }
};

}  // namespace series_data
