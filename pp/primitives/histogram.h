#pragma once

#include "label_set.h"
#include "primitives.h"

namespace PromPP::Primitives {

enum class CounterResetHint : uint8_t {
  kUnknown = 0,
  kReset,
  kNotReset,
  kGaugeType,
};

enum class HistogramType : uint8_t {
  kInt = 0,
  kFloat,
};

struct HistogramSpan {
  int32_t offset{};
  uint32_t length{std::numeric_limits<uint32_t>::max()};

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_valid() const noexcept { return length != std::numeric_limits<uint32_t>::max(); }

  bool operator==(const HistogramSpan& other) const noexcept = default;
};

enum class ValueType : uint8_t {
  kUnknown = 0,
  kInt,
  kUint = kInt,
  kFloat,
};

union HistogramBucketValue {
  int64_t value;
  double float_value;

  bool operator==(const HistogramBucketValue& other) const noexcept { return value == other.value; }
};

union HistogramValue {
  uint64_t value;
  double float_value;

  bool operator==(const HistogramValue& other) const noexcept { return value == other.value; }
};

template <template <class> class SpanType, template <class> class BucketsType, template <class> class CustomValuesType>
struct BasicHistogram {
  Timestamp timestamp;
  double zero_threshold{};
  HistogramValue zero_count{};
  HistogramValue count{};
  double sum{};
  SpanType<HistogramSpan> positive_spans{};
  SpanType<HistogramSpan> negative_spans{};
  BucketsType<HistogramBucketValue> positive_buckets{};
  BucketsType<HistogramBucketValue> negative_buckets{};
  CustomValuesType<double> custom_values{};
  HistogramType type;
  int8_t schema;
  CounterResetHint counter_reset_hint{CounterResetHint::kUnknown};

  bool operator==(const BasicHistogram& a) const noexcept = default;
};

template <class LabelSetType,
          template <class> class HistogramContainer,
          template <class> class SpanType,
          template <class> class BucketsType,
          template <class> class CustomValuesType>
struct HistogramTimeseries {
  using Histogram = BasicHistogram<SpanType, BucketsType, CustomValuesType>;

  LabelSetType label_set;
  HistogramContainer<Histogram> histograms;

  bool operator==(const HistogramTimeseries& a) const noexcept = default;
};

}  // namespace PromPP::Primitives