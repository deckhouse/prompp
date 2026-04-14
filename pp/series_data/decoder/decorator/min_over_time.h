#pragma once

#include "over_time_func_iterator.h"

namespace series_data::decoder::decorator {

class FindMinElementInIterator {
 public:
  PROMPP_ALWAYS_INLINE SeekResult operator()(PromPP::Primitives::Timestamp, double value) noexcept {
    if (BareBones::Encoding::Gorilla::isstalenan(min_value_) || value < min_value_) {
      min_value_ = value;
      return SeekResult::kUpdateSample;
    }

    return SeekResult::kNext;
  }

  PROMPP_ALWAYS_INLINE void set_result(UniversalDecodeIterator& iterator) const {
    if (BareBones::Encoding::Gorilla::isstalenan(min_value_)) [[unlikely]] {
      iterator.invalidate_sample();
    }
  }

 private:
  double min_value_{BareBones::Encoding::Gorilla::STALE_NAN};
};

class FindMinElement {
 public:
  explicit FindMinElement(encoder::Sample& result) : min_(result) { min_.value = BareBones::Encoding::Gorilla::STALE_NAN; }

  PROMPP_ALWAYS_INLINE void operator()(const encoder::Sample& sample) const noexcept {
    if (BareBones::Encoding::Gorilla::isstalenan(min_.value) || sample.value < min_.value) {
      min_ = sample;
    }
  }

  PROMPP_ALWAYS_INLINE void set_result() const {
    if (BareBones::Encoding::Gorilla::isstalenan(min_.value)) [[unlikely]] {
      min_.timestamp = kInvalidTimestamp;
    }
  }

 private:
  encoder::Sample& min_;
};

using MinOverTimeIterator = OverTimeFuncIterator<FindMinElementInIterator>;

}  // namespace series_data::decoder::decorator