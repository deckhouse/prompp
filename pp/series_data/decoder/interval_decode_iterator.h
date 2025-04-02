#pragma once

#include "series_data/encoder/sample.h"
#include "traits.h"

namespace series_data::decoder {

template <class DecodeIterator, class DecodeIteratorSentinel>
class IntervalDecodeIterator : public DecodeIteratorTypeTrait {
 public:
  using Timestamp = PromPP::Primitives::Timestamp;

  static constexpr Timestamp kMinInterval = 1;

  IntervalDecodeIterator(DecodeIterator&& iterator, DecodeIteratorSentinel&& end, Timestamp interval, Timestamp lookback)
      : iterator_(std::move(iterator)), iterator_end_(std::move(end)), interval_(std::max(interval, kMinInterval)), lookback_(lookback) {
    if (iterator_ != iterator_end_) {
      timestamp_ = align_timestamp(iterator_->timestamp, interval_);
      advance_to_next_sample();
    }
  }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const noexcept { return sample_; }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const noexcept { return &sample_; }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const noexcept { return iterator_ == iterator_end_ && sample_.timestamp == kNoSample; }

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
  static constexpr auto kNoSample = std::numeric_limits<Timestamp>::min();

  DecodeIterator iterator_;
  [[no_unique_address]] DecodeIteratorSentinel iterator_end_;
  Timestamp interval_;
  Timestamp lookback_;
  Timestamp timestamp_{};
  encoder::Sample sample_{.timestamp = kNoSample};

  PROMPP_ALWAYS_INLINE static Timestamp align_timestamp(Timestamp timestamp, Timestamp align_to) noexcept {
    const auto result = timestamp + align_to - 1;
    return result - result % align_to;
  }

  PROMPP_ALWAYS_INLINE void advance_to_next_sample() noexcept {
    if (iterator_ == iterator_end_) [[unlikely]] {
      sample_.timestamp = kNoSample;
      return;
    }

    Timestamp previous_timestamp;
    do {
      advance_to_last_sample_in_interval();
      previous_timestamp = std::exchange(timestamp_, timestamp_ + interval_);
    } while (!in_lookback_interval(sample_.timestamp, previous_timestamp) && iterator_ != iterator_end_);
  }

  PROMPP_ALWAYS_INLINE void advance_to_last_sample_in_interval() noexcept {
    for (; iterator_ != iterator_end_ && timestamp_ >= iterator_->timestamp; ++iterator_) {
      if (in_lookback_interval(iterator_->timestamp, timestamp_)) [[likely]] {
        sample_ = *iterator_;
      }
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool in_lookback_interval(Timestamp timestamp, Timestamp deadline) const noexcept {
    return deadline <= lookback_ + timestamp;
  }
};

}  // namespace series_data::decoder