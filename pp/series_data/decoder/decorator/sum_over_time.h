#pragma once

#include "over_time_func_iterator.h"

namespace series_data::decoder::decorator {

class SumOfElements {
 public:
  explicit SumOfElements(encoder::Sample& sum, const PromPP::Primitives::TimeInterval& interval) : sum_(sum), interval_(interval) {}

  PROMPP_ALWAYS_INLINE void operator()(PromPP::Primitives::Timestamp timestamp, double value) noexcept {
    if (BareBones::Encoding::Gorilla::isstalenan(sum_.value)) [[unlikely]] {
      sum_.value = 0.0;
    }

    kahan_sum_inc(value, sum_.value, c_);
    sum_.timestamp = timestamp;
  }

  ~SumOfElements() {
    if (sum_.timestamp != kInvalidTimestamp) [[likely]] {
      if (!std::isinf(sum_.value)) [[likely]] {
        sum_.value += c_;
      }

      sum_.timestamp = interval_.max;
    }
  }

 private:
  encoder::Sample& sum_;
  const PromPP::Primitives::TimeInterval& interval_;
  double c_{};

  PROMPP_ALWAYS_INLINE void kahan_sum_inc(double inc, double& sum, double& c) noexcept {
    const auto t = sum + inc;
    if (std::isinf(t)) {
      c = 0;
    } else if (std::abs(sum) >= std::abs(inc)) {
      c += sum - t + inc;
    } else {
      c += inc - t + sum;
    }

    sum = t;
  }
};

template <class Iterator = UniversalDecodeIterator>
using SumOverTimeIterator = OverTimeFuncIterator<SumOfElements, Iterator, true>;

}  // namespace series_data::decoder::decorator