#pragma once

#include "over_time_func_iterator.h"

namespace series_data::decoder::decorator {

class LastOverStep {
 public:
  explicit LastOverStep(encoder::Sample& sample, const PromPP::Primitives::TimeInterval& interval) : sample_(sample), interval_(interval) {}

  PROMPP_ALWAYS_INLINE void operator()(PromPP::Primitives::Timestamp, double value) noexcept {
    sample_.value = value;
    has_value_ = true;
  }

  ~LastOverStep() {
    if (has_value_) [[likely]] {
      sample_.timestamp = interval_.max;
    }
  }

 private:
  encoder::Sample& sample_;
  const PromPP::Primitives::TimeInterval& interval_;
  bool has_value_{};
};

template <class Iterator = UniversalDecodeIterator>
using LastOverStepIterator = OverTimeFuncIterator<LastOverStep, Iterator, true>;
template <class Iterator = UniversalDecodeIterator>
using LastOverStepWithStaleNansIterator = OverTimeFuncIterator<LastOverStep, Iterator, false>;

}  // namespace series_data::decoder::decorator