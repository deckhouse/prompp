#pragma once

#include "over_time_func_iterator.h"

namespace series_data::decoder::decorator {

class ElementsCounter {
 public:
  explicit ElementsCounter(encoder::Sample& sample, const PromPP::Primitives::TimeInterval& interval) : sample_(sample), interval_(interval) {
    sample_.value = 0;
  }

  PROMPP_ALWAYS_INLINE void operator()(PromPP::Primitives::Timestamp, double) const noexcept { ++sample_.value; }

  ~ElementsCounter() {
    if (sample_.value != 0) [[likely]] {
      sample_.timestamp = interval_.max;
    }
  }

 private:
  encoder::Sample& sample_;
  const PromPP::Primitives::TimeInterval& interval_;
};

using CountOverTimeIterator = OverTimeFuncIterator<ElementsCounter, true>;

}  // namespace series_data::decoder::decorator