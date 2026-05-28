#pragma once

#include "primitives/primitives.h"
#include "series_data/decoder/universal_decode_iterator.h"

namespace series_data::decoder::decorator {

template <class Iterator = UniversalDecodeIterator>
class LookbackDeltaIterator {
 public:
  DECODE_ITERATOR_TYPE_TRAITS();

  LookbackDeltaIterator(Iterator&& iterator, PromPP::Primitives::Timestamp lookback_delta) : iterator_(std::move(iterator)), lookback_delta_(lookback_delta) {
    update_sample();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const PromPP::Primitives::TimeInterval& interval() const noexcept { return iterator_.interval(); }
  PROMPP_ALWAYS_INLINE void set_interval(const PromPP::Primitives::TimeInterval& interval) {
    iterator_.set_interval(interval);
    update_sample();
  }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const { return sample_; }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const { return &sample_; }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const { return sample_.timestamp == kInvalidTimestamp; }

  PROMPP_ALWAYS_INLINE LookbackDeltaIterator& operator++() {
    ++iterator_;
    update_sample();
    return *this;
  }

  PROMPP_ALWAYS_INLINE LookbackDeltaIterator operator++(int) {
    const auto result = *this;
    ++*this;
    return result;
  }

 protected:
  static constexpr auto kInvalidSample = encoder::Sample{.timestamp = kInvalidTimestamp, .value = BareBones::Encoding::Gorilla::STALE_NAN};

  encoder::Sample sample_{kInvalidSample};
  Iterator iterator_;
  PromPP::Primitives::Timestamp lookback_delta_;

  PROMPP_ALWAYS_INLINE void update_sample() noexcept {
    if (iterator_ != DecodeIteratorSentinel{}) [[likely]] {
      if (BareBones::Encoding::Gorilla::isstalenan(iterator_->value)) [[unlikely]] {
        sample_ = kInvalidSample;
      } else {
        sample_ = *iterator_;
      }
    } else if (!sample_in_lookback_delta_interval()) {
      sample_ = kInvalidSample;
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool sample_in_lookback_delta_interval() const noexcept {
    return sample_.timestamp > iterator_.interval().max - lookback_delta_;
  }
};

}  // namespace series_data::decoder::decorator