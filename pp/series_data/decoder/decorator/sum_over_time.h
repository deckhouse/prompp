#pragma once

#include "over_time_func_iterator.h"

namespace series_data::decoder::decorator {

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

class SumOfElements {
 public:
  explicit SumOfElements(encoder::Sample& sum) : sum_(sum) {
    sum_ = encoder::Sample{.timestamp = kInvalidTimestamp, .value = BareBones::Encoding::Gorilla::STALE_NAN};
  }

  PROMPP_ALWAYS_INLINE void operator()(PromPP::Primitives::Timestamp timestamp, double value) noexcept {
    if (BareBones::Encoding::Gorilla::isstalenan(sum_.value)) [[unlikely]] {
      sum_.value = 0.0;
    }

    kahan_sum_inc(value, sum_.value, c_);
    sum_.timestamp = timestamp;
  }

  ~SumOfElements() {
    if (!std::isinf(sum_.value)) [[likely]] {
      sum_.value += c_;
    }
  }

 private:
  encoder::Sample& sum_;
  double c_{};
};

using SumOverTimeIterator = OverTimeFuncIterator<SumOfElements>;

}  // namespace series_data::decoder::decorator