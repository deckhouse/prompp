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

class SumOfElementsInIterator {
 public:
  PROMPP_ALWAYS_INLINE SeekResult operator()(PromPP::Primitives::Timestamp timestamp, double value) noexcept {
    if (BareBones::Encoding::Gorilla::isstalenan(sum_.value)) [[unlikely]] {
      sum_.value = 0.0;
    }

    kahan_sum_inc(value, sum_.value, c_);
    sum_.timestamp = timestamp;
    return SeekResult::kNext;
  }

  PROMPP_ALWAYS_INLINE void set_result(UniversalDecodeIterator& iterator) {
    if (BareBones::Encoding::Gorilla::isstalenan(sum_.value)) [[unlikely]] {
      iterator.invalidate_sample();
    } else {
      if (!std::isinf(sum_.value)) [[likely]] {
        sum_.value += c_;
      }
      iterator.set(sum_);
    }
  }

 private:
  encoder::Sample sum_{.value = BareBones::Encoding::Gorilla::STALE_NAN};
  double c_{};
};

class SumOfElements {
 public:
  explicit SumOfElements(encoder::Sample& result) : sum_(result) { sum_.value = BareBones::Encoding::Gorilla::STALE_NAN; }

  PROMPP_ALWAYS_INLINE void operator()(const encoder::Sample& sample) noexcept {
    if (BareBones::Encoding::Gorilla::isstalenan(sum_.value)) [[unlikely]] {
      sum_.value = 0.0;
    }

    kahan_sum_inc(sample.value, sum_.value, c_);
    sum_.timestamp = sample.timestamp;
  }

  PROMPP_ALWAYS_INLINE void set_result() const {
    if (BareBones::Encoding::Gorilla::isstalenan(sum_.value)) [[unlikely]] {
      sum_.timestamp = kInvalidTimestamp;
    }

    sum_.value += c_;
  }

 private:
  encoder::Sample& sum_;
  double c_{};
};

using SumOverTimeIterator = OverTimeFuncIterator<SumOfElementsInIterator>;

}  // namespace series_data::decoder::decorator