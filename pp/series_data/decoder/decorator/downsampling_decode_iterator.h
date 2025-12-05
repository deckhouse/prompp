#pragma once

#include "series_data/decoder/traits.h"
#include "series_data/encoder/sample.h"

namespace series_data::decoder::decorator {

template <class DecodeIterator>
class DownsamplingDecodeIterator {
 public:
  using Timestamp = PromPP::Primitives::Timestamp;

  DECODE_ITERATOR_TYPE_TRAITS();

  explicit DownsamplingDecodeIterator(Timestamp interval) : DownsamplingDecodeIterator(DecodeIterator{}, interval) {}
  DownsamplingDecodeIterator(DecodeIterator&& iterator, Timestamp interval) : iterator_(std::move(iterator)), interval_(interval) {
    advance_to_next_sample<false>();
  }

  PROMPP_ALWAYS_INLINE DownsamplingDecodeIterator& operator=(DecodeIterator&& iterator) noexcept {
    iterator_ = std::move(iterator);
    timestamp_ = kInvalidTimestamp;
    has_value_ = true;
    advance_to_next_sample<false>();
    return *this;
  }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const noexcept { return iterator_.operator*(); }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const noexcept { return iterator_.operator->(); }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const noexcept { return !has_value_; }

  PROMPP_ALWAYS_INLINE DownsamplingDecodeIterator& operator++() noexcept {
    advance_to_next_sample<true>();
    return *this;
  }

  PROMPP_ALWAYS_INLINE DownsamplingDecodeIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

 private:
  static constexpr Timestamp kInvalidTimestamp = std::numeric_limits<Timestamp>::min();
  static constexpr Timestamp kNoDownsampling = 0;

  bool has_value_{true};
  DecodeIterator iterator_;
  Timestamp interval_;
  Timestamp timestamp_{kInvalidTimestamp};

  PROMPP_ALWAYS_INLINE static Timestamp round_up_to_step(Timestamp timestamp, Timestamp step) noexcept {
    const auto result = timestamp + step - 1;
    return result - result % step;
  }

  template <bool MoveIterator>
  PROMPP_ALWAYS_INLINE void advance_to_next_sample() noexcept {
    if (interval_ == kNoDownsampling) {
      if constexpr (MoveIterator) {
        has_value_ = ++iterator_ != DecodeIteratorSentinel{};
      }
      return;
    }

    advance_to_last_sample_in_interval();
  }

  PROMPP_ALWAYS_INLINE void advance_to_last_sample_in_interval() noexcept {
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