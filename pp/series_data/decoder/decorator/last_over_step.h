#pragma once

#include "over_time_func_iterator.h"

namespace series_data::decoder::decorator {

class LastOverStep {
 public:
  explicit LastOverStep(encoder::Sample& sample, const PromPP::Primitives::TimeInterval& interval) : sample_(sample), interval_(interval) {}

  PROMPP_ALWAYS_INLINE void operator()(PromPP::Primitives::Timestamp timestamp, double value) const noexcept {
    sample_.value = value;
    sample_.timestamp = timestamp;
  }

  ~LastOverStep() {
    if (sample_.timestamp != kInvalidTimestamp) [[likely]] {
      sample_.timestamp = interval_.max - 1;
    }
  }

 private:
  encoder::Sample& sample_;
  const PromPP::Primitives::TimeInterval& interval_;
};

using LastOverStepIterator = OverTimeFuncIterator<LastOverStep, true>;
using LastOverStepWithStaleNansIterator = OverTimeFuncIterator<LastOverStep, false>;

}  // namespace series_data::decoder::decorator