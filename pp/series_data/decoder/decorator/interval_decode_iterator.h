#pragma once

#include "series_data/decoder/traits.h"
#include "series_data/encoder/sample.h"

namespace series_data::decoder::decorator {

template <class DecodeIterator, class DecodeIteratorSentinel>
class IntervalDecodeIterator : public DecodeIteratorTypeTrait {
 public:
  using Timestamp = PromPP::Primitives::Timestamp;

  IntervalDecodeIterator(DecodeIterator&& iterator, DecodeIteratorSentinel&& end, Timestamp interval)
      : iterator_(std::move(iterator)), iterator_end_(std::move(end)), interval_(std::max(interval, kMinInterval)) {
    advance_to_next_sample();
  }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const noexcept { return iterator_.operator*(); }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const noexcept { return iterator_.operator->(); }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const noexcept { return iterator_ == iterator_end_ && !has_value_; }

  PROMPP_ALWAYS_INLINE IntervalDecodeIterator& operator++() noexcept {
    advance_to_next_sample();
    return *this;
  }

  PROMPP_ALWAYS_INLINE IntervalDecodeIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

 private:
  static constexpr Timestamp kMinInterval = 1;

  DecodeIterator iterator_;
  [[no_unique_address]] DecodeIteratorSentinel iterator_end_;
  Timestamp interval_;
  Timestamp timestamp_{std::numeric_limits<Timestamp>::min()};
  bool has_value_{};

  PROMPP_ALWAYS_INLINE static Timestamp round_up_to_step(Timestamp timestamp, Timestamp step) noexcept {
    const auto result = timestamp + step - 1;
    return result - result % step;
  }

  PROMPP_ALWAYS_INLINE void advance_to_next_sample() noexcept {
    has_value_ = false;

    iterator_.seek([this](Timestamp timestamp) noexcept {
      if (timestamp > timestamp_) {
        if (has_value_) [[likely]] {
          return SeekResult::kStop;
        }

        timestamp_ = round_up_to_step(timestamp, interval_);
      }

      has_value_ = true;
      return SeekResult::kUpdateSample;
    });
  }
};

}  // namespace series_data::decoder::decorator