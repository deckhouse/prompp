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
  int32_t offset;
  uint32_t length;

  bool operator==(const HistogramSpan& other) const noexcept = default;
};

template <class LabelSetType, template <class> class SpanType, template <class> class BucketsType, template <class> class CustomValuesType>
struct BasicHistogram {
  union BucketValue {
    int64_t value;
    double float_value;

    bool operator==(const BucketValue& other) const noexcept { return value == other.value; }
  };
  union Value {
    uint64_t value;
    double float_value;

    bool operator==(const Value& other) const noexcept { return value == other.value; }
  };

  LabelSetType label_set;
  Timestamp timestamp;
  double zero_threshold;
  Value zero_count;
  Value count;
  double sum;
  SpanType<HistogramSpan> positive_spans;
  SpanType<HistogramSpan> negative_spans;
  BucketsType<BucketValue> positive_buckets;
  BucketsType<BucketValue> negative_buckets;
  CustomValuesType<double> custom_values;
  HistogramType type;
  int8_t schema;
  CounterResetHint counter_reset_hint;

  bool operator==(const BasicHistogram& a) const noexcept = default;
};

using Histogram = BasicHistogram<LabelSet, BareBones::Vector, BareBones::Vector, BareBones::Vector>;

}  // namespace PromPP::Primitives