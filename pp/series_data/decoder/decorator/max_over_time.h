#pragma once

#include "over_time_func_iterator.h"

namespace series_data::decoder::decorator {

class FindMaxElement {
 public:
  PROMPP_ALWAYS_INLINE SeekResult operator()(PromPP::Primitives::Timestamp, double value) noexcept {
    if (BareBones::Encoding::Gorilla::isstalenan(max_value_) || value > max_value_) {
      max_value_ = value;
      return SeekResult::kUpdateSample;
    }

    return SeekResult::kNext;
  }

  PROMPP_ALWAYS_INLINE void set_result(UniversalDecodeIterator& iterator) const {
    if (BareBones::Encoding::Gorilla::isstalenan(max_value_)) [[unlikely]] {
      iterator.invalidate_sample();
    }
  }

 private:
  double max_value_{BareBones::Encoding::Gorilla::STALE_NAN};
};

using MaxOverTimeIterator = OverTimeFuncIterator<FindMaxElement>;

}  // namespace series_data::decoder::decorator