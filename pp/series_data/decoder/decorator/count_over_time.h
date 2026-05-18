#pragma once

#include "over_time_func_iterator.h"

namespace series_data::decoder::decorator {

class ElementsCounter {
 public:
  explicit ElementsCounter(encoder::Sample& sample, const PromPP::Primitives::TimeInterval& interval) : sample_(sample), interval_(interval) {
    sample_.value = 0;
  }

  PROMPP_ALWAYS_INLINE void operator()(PromPP::Primitives::Timestamp timestamp, double) const noexcept {
    ++sample_.value;
    sample_.timestamp = timestamp;
  }

  ~ElementsCounter() {
    if (sample_.timestamp != kInvalidTimestamp) [[likely]] {
      sample_.timestamp = interval_.max - 1;
    }
  }

 private:
  encoder::Sample& sample_;
  const PromPP::Primitives::TimeInterval& interval_;
};

using CountOverTimeIterator = OverTimeFuncIterator<ElementsCounter, true>;

}  // namespace series_data::decoder::decorator