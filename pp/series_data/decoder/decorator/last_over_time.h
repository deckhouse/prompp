#pragma once

#include "over_time_func_iterator.h"

namespace series_data::decoder::decorator {

class FindLastElement {
 public:
  PROMPP_ALWAYS_INLINE SeekResult operator()(PromPP::Primitives::Timestamp, double) noexcept {
    has_value_ = true;
    return SeekResult::kUpdateSample;
  }

  PROMPP_ALWAYS_INLINE void set_result(UniversalDecodeIterator& iterator) const {
    if (!has_value_) [[unlikely]] {
      iterator.invalidate();
    }
  }

 private:
  bool has_value_{};
};

using LastOverTimeIterator = OverTimeFuncIterator<FindLastElement>;

}  // namespace series_data::decoder::decorator