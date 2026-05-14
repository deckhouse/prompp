#pragma once

#include "over_time_func_iterator.h"

namespace series_data::decoder::decorator {

class FindMaxElement {
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

using MaxOverTimeIterator = OverTimeFuncIterator<FindMaxElement>;

}  // namespace series_data::decoder::decorator