#pragma once

#include "over_time_func_iterator.h"

namespace series_data::decoder::decorator {

class ElementsCounter {
 public:
  explicit ElementsCounter(encoder::Sample& sample) : sample_(sample) { sample_.value = 0; }

  PROMPP_ALWAYS_INLINE void operator()(PromPP::Primitives::Timestamp timestamp, double) const noexcept {
    ++sample_.value;
    sample_.timestamp = timestamp;
  }

 private:
  encoder::Sample& sample_;
};

using CountOverTimeIterator = OverTimeFuncIterator<ElementsCounter>;

}  // namespace series_data::decoder::decorator