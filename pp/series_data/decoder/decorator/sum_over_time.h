#pragma once

#include "primitives/primitives.h"
#include "series_data/decoder/universal_decode_iterator.h"

namespace series_data::decoder::decorator {

class SumOverTimeIterator {
 public:
  DECODE_ITERATOR_TYPE_TRAITS();

  explicit SumOverTimeIterator(PromPP::Primitives::TimeInterval interval) : interval_(interval) {}
  explicit SumOverTimeIterator(UniversalDecodeIterator&& iterator, PromPP::Primitives::TimeInterval interval) : iterator_(iterator), interval_(interval) {
    calculate_sum();
  }

  PROMPP_ALWAYS_INLINE SumOverTimeIterator& operator=(UniversalDecodeIterator&& iterator) {
    iterator_ = std::move(iterator);
    calculate_sum();
    return *this;
  }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const { return iterator_.operator*(); }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const { return iterator_.operator->(); }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const { return iterator_->timestamp == kInvalidTimestamp; }

  PROMPP_ALWAYS_INLINE SumOverTimeIterator& operator++() {
    iterator_.invalidate();
    return *this;
  }

  PROMPP_ALWAYS_INLINE SumOverTimeIterator operator++(int) {
    const auto result = *this;
    ++*this;
    return result;
  }

 private:
  UniversalDecodeIterator iterator_;
  PromPP::Primitives::TimeInterval interval_;

  PROMPP_ALWAYS_INLINE void calculate_sum() {
    encoder::Sample sum{.value = BareBones::Encoding::Gorilla::STALE_NAN};
    double c{};

    iterator_.seek([&sum, &c, this](PromPP::Primitives::Timestamp timestamp, double value) {
      if (timestamp < interval_.min) {
        return SeekResult::kNext;
      }
      if (timestamp > interval_.max) {
        return SeekResult::kStop;
      }

      if (BareBones::Encoding::Gorilla::isstalenan(value)) [[unlikely]] {
        return SeekResult::kNext;
      }

      if (BareBones::Encoding::Gorilla::isstalenan(sum.value)) [[unlikely]] {
        sum.value = 0.0;
      }

      kahan_sum_inc(value, sum.value, c);
      sum.timestamp = timestamp;
      return SeekResult::kNext;
    });

    if (BareBones::Encoding::Gorilla::isstalenan(sum.value)) [[unlikely]] {
      iterator_.invalidate();
    } else {
      iterator_.set(sum);
    }
  }

  PROMPP_ALWAYS_INLINE static void kahan_sum_inc(double inc, double& sum, double& c) noexcept {
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

}  // namespace series_data::decoder::decorator