#pragma once

#include "over_time_func_iterator.h"

namespace series_data::decoder::decorator {

class FindMinElementInIterator {
 public:
  explicit FindMinElementInIterator(encoder::Sample& sample, const PromPP::Primitives::TimeInterval&) : sample_(sample) {}

  PROMPP_ALWAYS_INLINE void operator()(PromPP::Primitives::Timestamp timestamp, double value) const noexcept {
    if (BareBones::Encoding::Gorilla::isstalenan(sample_.value) || value < sample_.value) {
      sample_.value = value;
      sample_.timestamp = timestamp;
    }
  }

 private:
  encoder::Sample& sample_;
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

using MinOverTimeIterator = OverTimeFuncIterator<FindMinElementInIterator, true>;

}  // namespace series_data::decoder::decorator