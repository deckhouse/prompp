#pragma once

#include <algorithm>
#include <cmath>
#include <type_traits>

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

  template <class Value>
  PROMPP_ALWAYS_INLINE Value get() const noexcept {
    if constexpr (std::is_same_v<Value, double>) {
      return float_value;
    } else {
      return value;
    }
  }

  template <class Value>
  PROMPP_ALWAYS_INLINE void inc(Value inc_to) noexcept {
    if constexpr (std::is_same_v<Value, double>) {
      float_value += inc_to;
    } else {
      value += inc_to;
    }
  }

  template <class Value>
  PROMPP_ALWAYS_INLINE void set(Value new_value) noexcept {
    if constexpr (std::is_same_v<Value, double>) {
      float_value = new_value;
    } else {
      value = new_value;
    }
  }

  bool operator==(const HistogramBucketValue& other) const noexcept { return value == other.value; }
};

union HistogramValue {
  uint64_t value;
  double float_value;

  bool operator==(const HistogramValue& other) const noexcept { return value == other.value; }
};

struct ClassicHistogramBucket {
  HistogramValue cumulative_count{};
  double upper_bound{};
  HistogramValueType type{HistogramValueType::kUnknown};

  PROMPP_ALWAYS_INLINE double float_cumulative_count() const noexcept {
    return type == HistogramValueType::kFloat ? cumulative_count.float_value : static_cast<double>(cumulative_count.value);
  }
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
  HistogramType type{HistogramType::kInt};
  int8_t schema{};

  bool operator==(const BasicHistogram& a) const noexcept = default;

  template <class BucketsContainer>
  void convert_to_native(BucketsContainer& classic_buckets) {
    if (!positive_buckets.empty() || !positive_spans.empty()) [[unlikely]] {
      return;
    }

    prepare_classic_buckets(classic_buckets);
    const double total_count = append_infinity_bucket(classic_buckets);

    schema = kCustomBucketsSchema;
    positive_spans.emplace_back(HistogramSpan{.offset = 0, .length = static_cast<uint32_t>(classic_buckets.size())});
    positive_buckets.reserve(classic_buckets.size());
    fill_custom_values(classic_buckets);

    if (type == HistogramType::kFloat || !all_counts_are_integers(classic_buckets, total_count)) {
      fill_float_buckets(classic_buckets, total_count);
    } else {
      fill_int_buckets(classic_buckets, total_count);
    }
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
  template <class BucketsContainer>
  PROMPP_ALWAYS_INLINE static void prepare_classic_buckets(BucketsContainer& classic_buckets) {
    std::ranges::sort(classic_buckets, [](const auto& a, const auto& b) { return a.upper_bound < b.upper_bound; });

    const auto unique_end =
        std::unique(classic_buckets.begin(), classic_buckets.end(), [](const auto& a, const auto& b) { return a.upper_bound == b.upper_bound; });
    classic_buckets.erase(unique_end, classic_buckets.end());
  }

  template <class BucketsContainer>
  PROMPP_ALWAYS_INLINE double append_infinity_bucket(BucketsContainer& buckets) const {
    constexpr double inf = std::numeric_limits<double>::infinity();

    double total_count = type == HistogramType::kFloat ? count.float_value : static_cast<double>(count.value);
    if (total_count == 0 && !buckets.empty()) {
      total_count = buckets.back().float_cumulative_count();
    }

    if (buckets.empty() || buckets.back().upper_bound != inf) {
      buckets.emplace_back(ClassicHistogramBucket{
          .cumulative_count = {.float_value = total_count},
          .upper_bound = inf,
          .type = HistogramValueType::kFloat,
      });
    }
    return total_count;
  }

  template <class BucketsContainer>
  PROMPP_ALWAYS_INLINE static bool all_counts_are_integers(const BucketsContainer& buckets, double total_count) {
    if (total_count != std::round(total_count)) {
      return false;
    }

    return std::ranges::all_of(buckets, [](const auto& bucket) { return bucket.float_cumulative_count() == std::round(bucket.float_cumulative_count()); });
  }

  template <class BucketsContainer>
  PROMPP_ALWAYS_INLINE void fill_custom_values(const BucketsContainer& buckets) {
    custom_values.clear();
    if (buckets.size() <= 1) {
      return;
    }

    custom_values.resize(buckets.size() - 1);
    for (size_t i = 0; i < buckets.size() - 1; ++i) {
      if (!std::isinf(buckets[i].upper_bound)) {
        custom_values[i] = buckets[i].upper_bound;
      }
    }
  }

  template <class BucketsContainer>
  PROMPP_ALWAYS_INLINE void fill_float_buckets(const BucketsContainer& buckets, double total_count) {
    type = HistogramType::kFloat;
    count.float_value = total_count;

    double prev_count = 0;
    for (const auto& bucket : buckets) {
      positive_buckets.emplace_back().float_value = bucket.float_cumulative_count() - prev_count;
      prev_count = bucket.float_cumulative_count();
    }
    compact_float_histogram_buckets(positive_buckets, positive_spans, 0);
  }

  template <class BucketsContainer>
  PROMPP_ALWAYS_INLINE void fill_int_buckets(const BucketsContainer& buckets, double total_count) {
    type = HistogramType::kInt;
    count.value = static_cast<uint64_t>(std::round(total_count));

    int64_t prev_count = 0;
    int64_t prev_delta = 0;
    for (const auto& bucket : buckets) {
      const auto cumulative_count = static_cast<int64_t>(std::round(bucket.float_cumulative_count()));
      const auto delta = cumulative_count - prev_count;
      positive_buckets.emplace_back().value = delta - prev_delta;
      prev_count = cumulative_count;
      prev_delta = delta;
    }
    compact_int_histogram_buckets(positive_buckets, positive_spans, 2);
  }

  PROMPP_ALWAYS_INLINE static void compact_int_histogram_buckets(BareBones::Vector<HistogramBucketValue>& buckets,
                                                                 BareBones::Vector<HistogramSpan>& spans,
                                                                 int max_empty_buckets) {
    compact_buckets_impl<int64_t, true>(buckets, spans, max_empty_buckets);
  }

  PROMPP_ALWAYS_INLINE static void compact_float_histogram_buckets(BareBones::Vector<HistogramBucketValue>& buckets,
                                                                   BareBones::Vector<HistogramSpan>& spans,
                                                                   int max_empty_buckets) {
    compact_buckets_impl<double, false>(buckets, spans, max_empty_buckets);
  }

  // Folds a bucket into the running absolute count: int histograms store deltas, float histograms store absolutes.
  template <class BucketCount, bool delta_buckets>
  PROMPP_ALWAYS_INLINE static void advance_absolute(BucketCount& absolute, const HistogramBucketValue& bucket) noexcept {
    if constexpr (delta_buckets) {
      absolute += bucket.get<BucketCount>();
    } else {
      absolute = bucket.get<BucketCount>();
    }
  }

  // Port of model/histogram/generic.go compactBuckets: removes empty buckets and merges spans in place.
  template <class BucketCount, bool delta_buckets>
  PROMPP_ALWAYS_INLINE static void compact_buckets_impl(BareBones::Vector<HistogramBucketValue>& buckets,
                                                        BareBones::Vector<HistogramSpan>& spans,
                                                        int max_empty_buckets) {
    if (!needs_compaction<BucketCount, delta_buckets>(buckets, spans, max_empty_buckets)) {
      return;
    }

    merge_touching_spans(spans);
    drop_empty_spans(spans);
    if (spans.empty()) {
      buckets.clear();
      return;
    }

    remove_empty_buckets<BucketCount, delta_buckets>(buckets, spans, max_empty_buckets);

    if (max_empty_buckets == 0 || buckets.empty()) {
      return;
    }
    merge_small_gaps<BucketCount, delta_buckets>(buckets, spans, max_empty_buckets);
  }

  // Compaction is worthwhile only if there is an empty bucket to drop, or a span gap small enough to bridge.
  template <class BucketCount, bool delta_buckets>
  PROMPP_ALWAYS_INLINE static bool needs_compaction(const BareBones::Vector<HistogramBucketValue>& buckets,
                                                    const BareBones::Vector<HistogramSpan>& spans,
                                                    int max_empty_buckets) noexcept {
    BucketCount absolute{};
    for (const auto& bucket : buckets) {
      advance_absolute<BucketCount, delta_buckets>(absolute, bucket);
      if (absolute == BucketCount{}) {
        return true;
      }
    }

    for (const auto& span : spans) {
      if (static_cast<int>(span.offset) <= max_empty_buckets || span.length == 0) {
        return true;
      }
    }
    return false;
  }

  // Merges each span that directly abuts its predecessor (offset == 0) into it.
  PROMPP_ALWAYS_INLINE static void merge_touching_spans(BareBones::Vector<HistogramSpan>& spans) noexcept {
    if (spans.size() <= 1) {
      return;
    }

    size_t last = 0;
    for (size_t i = 1; i < spans.size(); ++i) {
      if (spans[i].offset == 0) {
        spans[last].length += spans[i].length;
        continue;
      }
      ++last;
      if (i != last) {
        spans[last] = spans[i];
      }
    }
    spans.resize(last + 1);
  }

  // Drops zero-length spans, folding their offset into the following span.
  PROMPP_ALWAYS_INLINE static void drop_empty_spans(BareBones::Vector<HistogramSpan>& spans) noexcept {
    size_t last = 0;
    for (size_t i = 0; i < spans.size(); ++i) {
      if (spans[i].length == 0) {
        if (i + 1 < spans.size()) {
          spans[i + 1].offset += spans[i].offset;
        }
        continue;
      }
      if (i != last) {
        spans[last] = spans[i];
      }
      ++last;
    }
    spans.resize(last);
  }

  // Walks buckets alongside spans, erasing runs of empty buckets and splitting/shrinking spans accordingly.
  template <class BucketCount, bool delta_buckets>
  PROMPP_ALWAYS_INLINE static void remove_empty_buckets(BareBones::Vector<HistogramBucketValue>& buckets,
                                                        BareBones::Vector<HistogramSpan>& spans,
                                                        int max_empty_buckets) {
    int i_bucket = 0;
    int i_span = 0;
    uint32_t pos_in_span = 0;
    BucketCount current_bucket_absolute{};

    const auto empty_buckets_here = [&]() -> int {
      int count = 0;
      auto abs = current_bucket_absolute;
      while (static_cast<uint32_t>(count) + pos_in_span < spans[static_cast<size_t>(i_span)].length && abs == BucketCount{}) {
        ++count;
        if (i_bucket + count >= static_cast<int>(buckets.size())) {
          break;
        }
        abs = buckets[static_cast<size_t>(i_bucket + count)].get<BucketCount>();
      }
      return count;
    };

    while (i_bucket < static_cast<int>(buckets.size()) && i_span < static_cast<int>(spans.size())) {
      advance_absolute<BucketCount, delta_buckets>(current_bucket_absolute, buckets[static_cast<size_t>(i_bucket)]);

      const int n_empty = empty_buckets_here();
      if (n_empty == 0) {
        ++i_bucket;
        ++pos_in_span;
        if (pos_in_span >= spans[static_cast<size_t>(i_span)].length) {
          pos_in_span = 0;
          ++i_span;
        }
        continue;
      }

      // A short interior gap is within budget: keep it and skip past it.
      if (pos_in_span > 0 && n_empty < static_cast<int>(spans[static_cast<size_t>(i_span)].length - pos_in_span) && n_empty <= max_empty_buckets) {
        i_bucket += n_empty;
        if constexpr (delta_buckets) {
          current_bucket_absolute = {};
        }
        pos_in_span += static_cast<uint32_t>(n_empty);
        continue;
      }

      // Carry the removed delta over to the next surviving bucket so absolute counts stay correct.
      if constexpr (delta_buckets) {
        if (i_bucket + n_empty < static_cast<int>(buckets.size())) {
          const auto current_delta = buckets[static_cast<size_t>(i_bucket)].get<BucketCount>();
          current_bucket_absolute = -current_delta;
          buckets[static_cast<size_t>(i_bucket + n_empty)].inc(current_delta);
        }
      }

      buckets.erase(buckets.begin() + i_bucket, buckets.begin() + i_bucket + n_empty);
      shrink_spans_after_removal(spans, i_span, pos_in_span, n_empty);
    }
  }

  // Fixes up the span list after n_empty buckets were erased at the cursor (i_span, pos_in_span), advancing the
  // cursor when the erased run splits the current span.
  PROMPP_ALWAYS_INLINE static void shrink_spans_after_removal(BareBones::Vector<HistogramSpan>& spans,
                                                              int& i_span,
                                                              uint32_t& pos_in_span,
                                                              int n_empty) noexcept {
    HistogramSpan& span = spans[static_cast<size_t>(i_span)];

    if (pos_in_span == 0) {
      if (n_empty == static_cast<int>(span.length)) {
        // The whole span vanished: drop it and hand its offset plus the gap to the following span.
        const int32_t carried_offset = span.offset + n_empty;
        spans.erase(spans.begin() + i_span, spans.begin() + i_span + 1);
        if (i_span < static_cast<int>(spans.size())) {
          spans[static_cast<size_t>(i_span)].offset += carried_offset;
        }
        return;
      }
      // The gap is at the span start: shrink the span and shift its offset past the gap.
      span.length -= static_cast<uint32_t>(n_empty);
      span.offset += n_empty;
      return;
    }

    // The gap is inside the span: keep the head as the current span and re-inject the tail as the next span.
    HistogramSpan tail_span{
        .offset = n_empty,
        .length = span.length - pos_in_span - static_cast<uint32_t>(n_empty),
    };
    span.length = pos_in_span;
    ++i_span;
    pos_in_span = 0;
    if (tail_span.length == 0) {
      if (i_span < static_cast<int>(spans.size())) {
        spans[static_cast<size_t>(i_span)].offset += n_empty;
      }
      return;
    }
    spans.insert(spans.begin() + i_span, std::move(tail_span));
  }

  // Bridges spans separated by a gap no larger than max_empty_buckets by materialising the intervening zero buckets.
  template <class BucketCount, bool delta_buckets>
  PROMPP_ALWAYS_INLINE static void merge_small_gaps(BareBones::Vector<HistogramBucketValue>& buckets,
                                                    BareBones::Vector<HistogramSpan>& spans,
                                                    int max_empty_buckets) {
    [[maybe_unused]] BucketCount current_bucket_absolute{};

    // Advances the running absolute count across `count` buckets starting at `from` (a no-op for float histograms).
    const auto accumulate_over = [&]([[maybe_unused]] int from, [[maybe_unused]] int count) {
      if constexpr (delta_buckets) {
        for (int i = 0; i < count; ++i) {
          current_bucket_absolute += buckets[static_cast<size_t>(from + i)].get<BucketCount>();
        }
      }
    };

    int i_bucket = static_cast<int>(spans[0].length);
    accumulate_over(0, i_bucket);

    int i_span = 1;
    while (i_span < static_cast<int>(spans.size())) {
      const auto& span = spans[static_cast<size_t>(i_span)];
      const int gap = span.offset;

      // Gap too wide to bridge: keep the span and walk the running count past its buckets.
      if (gap > max_empty_buckets) {
        const int length = static_cast<int>(span.length);
        accumulate_over(i_bucket, length);
        i_bucket += length;
        ++i_span;
        continue;
      }

      // Bridge the gap: fold this span into the previous one and materialise `gap` zero buckets between them.
      spans[static_cast<size_t>(i_span - 1)].length += static_cast<uint32_t>(gap) + span.length;
      spans.erase(spans.begin() + i_span, spans.begin() + i_span + 1);

      insert_zero_buckets<BucketCount, delta_buckets>(buckets, i_bucket, gap, current_bucket_absolute);
      i_bucket += gap;
      current_bucket_absolute = buckets[static_cast<size_t>(i_bucket)].get<BucketCount>();
    }
  }

  // Inserts `count` zero buckets at index `at` in place. For delta histograms the running absolute count is threaded
  // through the inserted region so the delta encoding stays consistent across the gap.
  template <class BucketCount, bool delta_buckets>
  PROMPP_ALWAYS_INLINE static void insert_zero_buckets(BareBones::Vector<HistogramBucketValue>& buckets,
                                                       int at,
                                                       int count,
                                                       [[maybe_unused]] BucketCount absolute) {
    const size_t old_size = buckets.size();
    buckets.resize(old_size + static_cast<size_t>(count));

    // Shift the tail right by `count`, back-to-front so unread source elements are never clobbered.
    for (size_t i = old_size; i-- > static_cast<size_t>(at);) {
      buckets[i + static_cast<size_t>(count)] = buckets[i];
    }

    for (int i = at; i < at + count; ++i) {
      buckets[static_cast<size_t>(i)] = HistogramBucketValue{};
    }

    if constexpr (delta_buckets) {
      buckets[static_cast<size_t>(at)].set(-absolute);
      buckets[static_cast<size_t>(at + count)].inc(absolute);
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

}  // namespace PromPP::Primitives