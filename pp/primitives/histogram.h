#pragma once

#include <algorithm>
#include <cmath>

#include "bare_bones/vector.h"
#include "primitives.h"

namespace PromPP::Primitives {

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

enum class HistogramValueType : uint8_t {
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

constexpr int8_t kCustomBucketsSchema = -53;

template <template <class> class SpanType, template <class> class BucketsType, template <class> class CustomValuesType>
struct BasicHistogram {
  Timestamp timestamp{};
  double zero_threshold{};
  HistogramValue zero_count{};
  HistogramValue count{};
  double sum{0.0};
  SpanType<HistogramSpan> positive_spans{};
  SpanType<HistogramSpan> negative_spans{};
  BucketsType<HistogramBucketValue> positive_buckets{};
  BucketsType<HistogramBucketValue> negative_buckets{};
  CustomValuesType<double> custom_values{};
  HistogramType type;
  int8_t schema{};

  bool operator==(const BasicHistogram& a) const noexcept = default;

  template <class BucketsContainer>
  void convert_to_native(BucketsContainer& classic_buckets) {
    if (!positive_buckets.empty() || !positive_spans.empty()) [[unlikely]] {
      return;
    }

    std::ranges::sort(classic_buckets, [](const auto& a, const auto& b) { return a.upper_bound < b.upper_bound; });

    const auto unique_end =
        std::unique(classic_buckets.begin(), classic_buckets.end(), [](const auto& a, const auto& b) { return a.upper_bound == b.upper_bound; });
    classic_buckets.erase(unique_end, classic_buckets.end());

    struct SortedBucket {
      double upper_bound{};
      double cumulative_count{};
    };

    BareBones::Vector<SortedBucket> buckets;
    buckets.reserve(classic_buckets.size());
    for (const auto& classic_bucket : classic_buckets) {
      buckets.emplace_back(classic_bucket.upper_bound, classic_histogram_bucket_cumulative_count(classic_bucket));
    }

    constexpr double inf = std::numeric_limits<double>::infinity();
    double total_count = type == HistogramType::kFloat ? count.float_value : static_cast<double>(count.value);

    if (total_count == 0 && !buckets.empty()) {
      total_count = buckets.back().cumulative_count;
    }

    if (buckets.empty() || buckets.back().upper_bound != inf) {
      buckets.emplace_back(SortedBucket{.upper_bound = inf, .cumulative_count = total_count});
    }

    bool use_float = type == HistogramType::kFloat;
    if (!use_float) {
      const auto rounded_total = std::round(total_count);
      if (total_count != rounded_total) {
        use_float = true;
      } else {
        for (const auto& bucket : buckets) {
          const auto rounded_count = std::round(bucket.cumulative_count);
          if (bucket.cumulative_count != rounded_count) {
            use_float = true;
            break;
          }
        }
      }
    }

    schema = kCustomBucketsSchema;
    positive_spans.emplace_back(HistogramSpan{.offset = 0, .length = static_cast<uint32_t>(buckets.size())});
    positive_buckets.reserve(buckets.size());
    custom_values.clear();

    if (buckets.size() > 1) {
      custom_values.resize(buckets.size() - 1);
      for (size_t i = 0; i < buckets.size() - 1; ++i) {
        if (!std::isinf(buckets[i].upper_bound)) {
          custom_values[i] = buckets[i].upper_bound;
        }
      }
    }

    if (use_float) {
      type = HistogramType::kFloat;
      count.float_value = total_count;
      double prev_count = 0;
      for (const auto& bucket : buckets) {
        positive_buckets.emplace_back().float_value = bucket.cumulative_count - prev_count;
        prev_count = bucket.cumulative_count;
      }
      compact_float_histogram_buckets(positive_buckets, positive_spans, 0);
      return;
    }

    type = HistogramType::kInt;
    count.value = static_cast<uint64_t>(std::round(total_count));

    int64_t prev_count = 0;
    int64_t prev_delta = 0;
    for (const auto& bucket : buckets) {
      const auto delta = static_cast<int64_t>(std::round(bucket.cumulative_count)) - prev_count;
      positive_buckets.emplace_back().value = delta - prev_delta;
      prev_count = static_cast<int64_t>(std::round(bucket.cumulative_count));
      prev_delta = delta;
    }
    compact_int_histogram_buckets(positive_buckets, positive_spans, 2);
  }

  void compact_buckets(int max_empty_buckets) {
    if (type == HistogramType::kFloat) {
      compact_float_histogram_buckets(positive_buckets, positive_spans, max_empty_buckets);
      compact_float_histogram_buckets(negative_buckets, negative_spans, max_empty_buckets);
    } else {
      compact_int_histogram_buckets(positive_buckets, positive_spans, max_empty_buckets);
      compact_int_histogram_buckets(negative_buckets, negative_spans, max_empty_buckets);
    }
  }

 private:
  PROMPP_ALWAYS_INLINE void compact_int_histogram_buckets(BareBones::Vector<HistogramBucketValue>& buckets,
                                                          BareBones::Vector<HistogramSpan>& spans,
                                                          int max_empty_buckets) {
    BareBones::Vector<int64_t> primary_buckets;
    primary_buckets.reserve(buckets.size());
    for (const auto& bucket : buckets) {
      primary_buckets.emplace_back(bucket.value);
    }

    compact_buckets_impl<int64_t, true>(primary_buckets, spans, max_empty_buckets);

    buckets.clear();
    buckets.reserve(primary_buckets.size());
    for (const auto value : primary_buckets) {
      buckets.emplace_back(HistogramBucketValue{.value = value});
    }
  }

  PROMPP_ALWAYS_INLINE void compact_float_histogram_buckets(BareBones::Vector<HistogramBucketValue>& buckets,
                                                            BareBones::Vector<HistogramSpan>& spans,
                                                            int max_empty_buckets) {
    BareBones::Vector<double> primary_buckets;
    primary_buckets.reserve(buckets.size());
    for (const auto& bucket : buckets) {
      primary_buckets.emplace_back(bucket.float_value);
    }

    compact_buckets_impl<double, false>(primary_buckets, spans, max_empty_buckets);

    buckets.clear();
    buckets.reserve(primary_buckets.size());
    for (const auto value : primary_buckets) {
      buckets.emplace_back(HistogramBucketValue{.float_value = value});
    }
  }

  template <class BucketCount, bool delta_buckets>
  PROMPP_ALWAYS_INLINE void compact_buckets_impl(BareBones::Vector<BucketCount>& primary_buckets,
                                                 BareBones::Vector<HistogramSpan>& spans,
                                                 int max_empty_buckets) {
    // Port of model/histogram/generic.go compactBuckets.
    bool nothing_to_do = true;
    BucketCount current_bucket_absolute{};

    for (const auto bucket : primary_buckets) {
      if constexpr (delta_buckets) {
        current_bucket_absolute += bucket;
      } else {
        current_bucket_absolute = bucket;
      }
      if (current_bucket_absolute == BucketCount{}) {
        nothing_to_do = false;
        break;
      }
    }

    if (nothing_to_do) {
      for (const auto& span : spans) {
        if (static_cast<int>(span.offset) <= max_empty_buckets || span.length == 0) {
          nothing_to_do = false;
          break;
        }
      }
      if (nothing_to_do) {
        return;
      }
    }

    int i_bucket = 0;
    int i_span = 0;
    uint32_t pos_in_span = 0;
    current_bucket_absolute = {};

    const auto empty_buckets_here = [&](int bucket_index, uint32_t position_in_span) -> int {
      int count = 0;
      auto abs = current_bucket_absolute;
      while (static_cast<uint32_t>(count) + position_in_span < spans[static_cast<size_t>(i_span)].length && abs == BucketCount{}) {
        ++count;
        if (bucket_index + count >= static_cast<int>(primary_buckets.size())) {
          break;
        }
        abs = primary_buckets[static_cast<size_t>(bucket_index + count)];
      }
      return count;
    };

    if (spans.size() > 1) {
      i_span = 0;
      for (size_t i = 1; i < spans.size(); ++i) {
        if (spans[i].offset == 0) {
          spans[static_cast<size_t>(i_span)].length += spans[i].length;
          continue;
        }
        ++i_span;
        if (i != static_cast<size_t>(i_span)) {
          spans[static_cast<size_t>(i_span)] = spans[i];
        }
      }
      spans.resize(static_cast<size_t>(i_span + 1));
      i_span = 0;
    }

    i_span = 0;
    for (size_t i = 0; i < spans.size(); ++i) {
      if (spans[i].length == 0) {
        if (i + 1 < spans.size()) {
          spans[i + 1].offset += spans[i].offset;
        }
        continue;
      }
      if (static_cast<int>(i) != i_span) {
        spans[static_cast<size_t>(i_span)] = spans[i];
      }
      ++i_span;
    }
    spans.resize(static_cast<size_t>(i_span));
    i_span = 0;

    if (spans.empty()) {
      primary_buckets.clear();
      return;
    }

    while (i_bucket < static_cast<int>(primary_buckets.size()) && i_span < static_cast<int>(spans.size())) {
      if constexpr (delta_buckets) {
        current_bucket_absolute += primary_buckets[static_cast<size_t>(i_bucket)];
      } else {
        current_bucket_absolute = primary_buckets[static_cast<size_t>(i_bucket)];
      }

      const int n_empty = empty_buckets_here(i_bucket, pos_in_span);
      if (n_empty > 0) {
        if (pos_in_span > 0 && n_empty < static_cast<int>(spans[static_cast<size_t>(i_span)].length - pos_in_span) && n_empty <= max_empty_buckets) {
          i_bucket += n_empty;
          if constexpr (delta_buckets) {
            current_bucket_absolute = {};
          }
          pos_in_span += static_cast<uint32_t>(n_empty);
          continue;
        }

        if constexpr (delta_buckets) {
          if (i_bucket + n_empty < static_cast<int>(primary_buckets.size())) {
            current_bucket_absolute = -primary_buckets[static_cast<size_t>(i_bucket)];
            primary_buckets[static_cast<size_t>(i_bucket + n_empty)] += primary_buckets[static_cast<size_t>(i_bucket)];
          }
        }

        primary_buckets.erase(primary_buckets.begin() + i_bucket, primary_buckets.begin() + i_bucket + n_empty);

        if (pos_in_span == 0) {
          if (n_empty == static_cast<int>(spans[static_cast<size_t>(i_span)].length)) {
            const auto offset = spans[static_cast<size_t>(i_span)].offset;
            spans.erase(spans.begin() + i_span, spans.begin() + i_span + 1);
            if (i_span < static_cast<int>(spans.size())) {
              spans[static_cast<size_t>(i_span)].offset += offset + static_cast<int32_t>(n_empty);
            }
            continue;
          }
          spans[static_cast<size_t>(i_span)].length -= static_cast<uint32_t>(n_empty);
          spans[static_cast<size_t>(i_span)].offset += static_cast<int32_t>(n_empty);
          continue;
        }

        HistogramSpan new_span{
            .offset = static_cast<int32_t>(n_empty),
            .length = spans[static_cast<size_t>(i_span)].length - pos_in_span - static_cast<uint32_t>(n_empty),
        };
        spans[static_cast<size_t>(i_span)].length = pos_in_span;
        ++i_span;
        pos_in_span = 0;
        if (new_span.length == 0) {
          if (i_span < static_cast<int>(spans.size())) {
            spans[static_cast<size_t>(i_span)].offset += static_cast<int32_t>(n_empty);
          }
          continue;
        }
        spans.insert(spans.begin() + i_span, std::move(new_span));
        continue;
      }

      ++i_bucket;
      ++pos_in_span;
      if (pos_in_span >= spans[static_cast<size_t>(i_span)].length) {
        pos_in_span = 0;
        ++i_span;
      }
    }

    if (max_empty_buckets == 0 || primary_buckets.empty()) {
      return;
    }

    i_bucket = static_cast<int>(spans[0].length);
    if constexpr (delta_buckets) {
      current_bucket_absolute = {};
      for (int i = 0; i < i_bucket; ++i) {
        current_bucket_absolute += primary_buckets[static_cast<size_t>(i)];
      }
    }

    i_span = 1;
    while (i_span < static_cast<int>(spans.size())) {
      if (static_cast<int>(spans[static_cast<size_t>(i_span)].offset) > max_empty_buckets) {
        const int length = static_cast<int>(spans[static_cast<size_t>(i_span)].length);
        if constexpr (delta_buckets) {
          for (int i = 0; i < length; ++i) {
            current_bucket_absolute += primary_buckets[static_cast<size_t>(i_bucket + i)];
          }
        }
        i_bucket += length;
        ++i_span;
        continue;
      }

      const int offset = static_cast<int>(spans[static_cast<size_t>(i_span)].offset);
      spans[static_cast<size_t>(i_span - 1)].length += static_cast<uint32_t>(offset) + spans[static_cast<size_t>(i_span)].length;
      spans.erase(spans.begin() + i_span, spans.begin() + i_span + 1);

      BareBones::Vector<BucketCount> new_primary_buckets;
      new_primary_buckets.resize(primary_buckets.size() + static_cast<size_t>(offset), BucketCount{});
      for (int i = 0; i < i_bucket; ++i) {
        new_primary_buckets[static_cast<size_t>(i)] = primary_buckets[static_cast<size_t>(i)];
      }
      for (size_t i = static_cast<size_t>(i_bucket); i < primary_buckets.size(); ++i) {
        new_primary_buckets[i + static_cast<size_t>(offset)] = primary_buckets[i];
      }
      if constexpr (delta_buckets) {
        new_primary_buckets[static_cast<size_t>(i_bucket)] = -current_bucket_absolute;
        new_primary_buckets[static_cast<size_t>(i_bucket + offset)] += current_bucket_absolute;
      }
      primary_buckets = std::move(new_primary_buckets);

      i_bucket += offset;
      current_bucket_absolute = primary_buckets[static_cast<size_t>(i_bucket)];
    }
  }
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

struct ClassicHistogramBucket {
  HistogramValue cumulative_count{};
  double upper_bound{};
  HistogramValueType type{HistogramValueType::kUnknown};
};

[[nodiscard]] PROMPP_ALWAYS_INLINE double classic_histogram_bucket_cumulative_count(const ClassicHistogramBucket& bucket) noexcept {
  if (bucket.type == HistogramValueType::kFloat) {
    return bucket.cumulative_count.float_value;
  }

  return static_cast<double>(bucket.cumulative_count.value);
}

}  // namespace PromPP::Primitives