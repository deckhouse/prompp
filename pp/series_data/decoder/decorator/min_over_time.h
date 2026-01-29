#pragma once

#include "over_time_func_iterator.h"

namespace series_data::decoder::decorator {

class FindMinElement {
 public:
  PROMPP_ALWAYS_INLINE SeekResult operator()(PromPP::Primitives::Timestamp, double value) noexcept {
    if (BareBones::Encoding::Gorilla::isstalenan(min_value_) || value < min_value_) {
      min_value_ = value;
      return SeekResult::kUpdateSample;
    }

    return SeekResult::kNext;
  }

  PROMPP_ALWAYS_INLINE void set_result(UniversalDecodeIterator& iterator) const {
    if (BareBones::Encoding::Gorilla::isstalenan(min_value_)) [[unlikely]] {
      iterator.invalidate();
    }
  }

 private:
  double min_value_{BareBones::Encoding::Gorilla::STALE_NAN};
};

using MinOverTimeIterator = OverTimeFuncIterator<FindMinElement>;

}  // namespace series_data::decoder::decorator