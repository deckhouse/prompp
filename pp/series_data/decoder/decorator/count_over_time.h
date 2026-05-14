#pragma once

#include "over_time_func_iterator.h"

namespace series_data::decoder::decorator {

class ElementsCounter {
 public:
  PROMPP_ALWAYS_INLINE SeekResult operator()(PromPP::Primitives::Timestamp timestamp, double) noexcept {
    ++sample_.value;
    sample_.timestamp = timestamp;
    return SeekResult::kNext;
  }

  PROMPP_ALWAYS_INLINE void set_result(UniversalDecodeIterator& iterator) const { iterator.set(sample_); }

 private:
  encoder::Sample sample_{.timestamp = kInvalidTimestamp, .value = 0};
};

using CountOverTimeIterator = OverTimeFuncIterator<ElementsCounter>;

}  // namespace series_data::decoder::decorator