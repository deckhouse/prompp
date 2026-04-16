#pragma once

#include "over_time_func_iterator.h"

namespace series_data::decoder::decorator {

class FindMaxElementInIterator {
 public:
  PROMPP_ALWAYS_INLINE SeekResult operator()(PromPP::Primitives::Timestamp, double value) noexcept {
    if (BareBones::Encoding::Gorilla::isstalenan(max_value_) || value > max_value_) {
      max_value_ = value;
      return SeekResult::kUpdateSample;
    }

    return SeekResult::kNext;
  }

  PROMPP_ALWAYS_INLINE void set_result(UniversalDecodeIterator& iterator) const {
    if (BareBones::Encoding::Gorilla::isstalenan(max_value_)) [[unlikely]] {
      iterator.invalidate_sample();
    }
  }

 private:
  double max_value_{BareBones::Encoding::Gorilla::STALE_NAN};
};

class FindMaxElement {
 public:
  explicit FindMaxElement(encoder::Sample& result) : max_(result) { max_.value = BareBones::Encoding::Gorilla::STALE_NAN; }

  PROMPP_ALWAYS_INLINE void operator()(const encoder::Sample& sample) const noexcept {
    if (BareBones::Encoding::Gorilla::isstalenan(max_.value) || sample.value > max_.value) {
      max_ = sample;
    }
  }

  PROMPP_ALWAYS_INLINE void set_result() const {
    if (BareBones::Encoding::Gorilla::isstalenan(max_.value)) [[unlikely]] {
      max_.timestamp = kInvalidTimestamp;
    }
  }

 private:
  encoder::Sample& max_;
};

using MaxOverTimeIterator = OverTimeFuncIterator<FindMaxElementInIterator>;

}  // namespace series_data::decoder::decorator