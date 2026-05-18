#pragma once

#include "over_time_func_iterator.h"

namespace series_data::decoder::decorator {

class LastOverTime {
 public:
  explicit LastOverTime(encoder::Sample& sample, const PromPP::Primitives::TimeInterval&) : sample_(sample) {}

  PROMPP_ALWAYS_INLINE void operator()(PromPP::Primitives::Timestamp timestamp, double value) const noexcept {
    sample_.value = value;
    sample_.timestamp = timestamp;
  }

 private:
  encoder::Sample& sample_;
};

using LastOverTimeIterator = OverTimeFuncIterator<LastOverTime>;

}  // namespace series_data::decoder::decorator