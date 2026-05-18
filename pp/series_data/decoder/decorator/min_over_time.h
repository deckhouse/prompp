#pragma once

#include "over_time_func_iterator.h"

namespace series_data::decoder::decorator {

class FindMinElement {
 public:
  explicit FindMinElement(encoder::Sample& sample, const PromPP::Primitives::TimeInterval&) : sample_(sample) {}

  PROMPP_ALWAYS_INLINE void operator()(PromPP::Primitives::Timestamp timestamp, double value) const noexcept {
    if (BareBones::Encoding::Gorilla::isstalenan(sample_.value) || value < sample_.value) {
      sample_.value = value;
      sample_.timestamp = timestamp;
    }
  }

 private:
  encoder::Sample& sample_;
};

using MinOverTimeIterator = OverTimeFuncIterator<FindMinElement>;

}  // namespace series_data::decoder::decorator