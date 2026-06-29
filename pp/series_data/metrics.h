#pragma once

#include <string>
#include <utility>

#include "common.h"
#include "metrics/storage.h"
#include "series_data/encoder/timestamp/encoder.h"

namespace series_data {

template <class Reallocator>
struct Metrics final : metrics::MetricsPage<Metrics<Reallocator>> {
  using metrics::MetricsPage<Metrics>::MetricsPage;

  // address_label is moved into the page and owned by it. Every metric below stores a non-owning view (Go::String) of the
  // label value, so the backing storage must outlive the page. Keeping the string inside the page (instead of in the
  // DataStorage that registered it) ties the value's lifetime to the page itself, which is reclaimed only by
  // metrics::Storage::remove_unused_pages(). This avoids a use-after-free where a concurrent scrape reads the label after
  // the owning DataStorage has already been destroyed.
  PROMPP_ALWAYS_INLINE explicit Metrics(std::string address_label)
      : metrics::MetricsPage<Metrics>(outdated_samples_count_),
        address_label_(std::move(address_label)),
        outdated_samples_count_{label_set(), "prompp_data_storage_outdated_samples_count"},
        outdated_chunks_count_{label_set(), "prompp_data_storage_outdated_chunks_count"},
        finalized_chunks_count_{label_set(), "prompp_data_storage_finalized_chunks_count"},
        timestamp_states_count_{label_set(), "prompp_data_storage_timestamp_states_count"},
        uint32_constants_count_{label_set(), "prompp_data_storage_uint32_constants_count"},
        float32_constants_count_{label_set(), "prompp_data_storage_float32_constants_count"},
        double_constants_count_{label_set(), "prompp_data_storage_double_constants_count"},
        two_double_constants_count_{label_set(), "prompp_data_storage_two_double_constants_count"},
        asc_int_count_{label_set(), "prompp_data_storage_asc_int_count"},
        asc_int_then_values_gorilla_count_{label_set(), "prompp_data_storage_asc_int_then_values_gorilla_count"},
        values_gorilla_count_{label_set(), "prompp_data_storage_values_gorilla_count"},
        gorilla_count_{label_set(), "prompp_data_storage_gorilla_count"} {}

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
  PROMPP_ALWAYS_INLINE metrics::Gauge& timestamp_states() noexcept { return timestamp_states_count_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE double timestamp_states_count() const noexcept { return timestamp_states_count_.value(); }

 private:
  const std::string address_label_;

  metrics::Counter outdated_samples_count_;
  metrics::Counter outdated_chunks_count_;
  metrics::Gauge finalized_chunks_count_;
  metrics::Gauge timestamp_states_count_;

  metrics::Gauge uint32_constants_count_;
  metrics::Gauge float32_constants_count_;
  metrics::Gauge double_constants_count_;
  metrics::Gauge two_double_constants_count_;
  metrics::Gauge asc_int_count_;
  metrics::Gauge asc_int_then_values_gorilla_count_;
  metrics::Gauge values_gorilla_count_;
  metrics::Gauge gorilla_count_;

  [[nodiscard]] PROMPP_ALWAYS_INLINE PromPP::Primitives::LabelViewSet label_set() const {
    return PromPP::Primitives::LabelViewSet{{"address", address_label_}};
  }

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

      default:
        assert(encoding_type != EncodingType::kUnknown);
        return std::forward<Handler>(handler)(metrics.gorilla_count_);
    }
  }
};

}  // namespace series_data
