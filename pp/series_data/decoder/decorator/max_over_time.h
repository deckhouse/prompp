#pragma once

#include "over_time_func_iterator.h"

namespace series_data::decoder::decorator {

class FindMaxElementInIterator {
 public:
  explicit FindMaxElement(encoder::Sample& sample) : sample_{sample} {
    sample_ = encoder::Sample{.timestamp = kInvalidTimestamp, .value = BareBones::Encoding::Gorilla::STALE_NAN};
  }

  PROMPP_ALWAYS_INLINE void operator()(PromPP::Primitives::Timestamp timestamp, double value) const noexcept {
    if (BareBones::Encoding::Gorilla::isstalenan(sample_.value) || value > sample_.value) {
      sample_.value = value;
      sample_.timestamp = timestamp;
    }
  }

 private:
  encoder::Sample& sample_;
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