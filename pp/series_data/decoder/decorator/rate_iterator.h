#pragma once

#include "primitives/primitives.h"
#include "series_data/decoder/universal_decode_iterator.h"

namespace series_data::decoder::decorator {

template <class Iterator = UniversalDecodeIterator>
class RateIterator {
 public:
  DECODE_ITERATOR_TYPE_TRAITS();

  explicit RateIterator(PromPP::Primitives::TimeInterval interval) : interval_(interval) {}
  explicit RateIterator(Iterator&& iterator, PromPP::Primitives::TimeInterval interval) : iterator_(std::move(iterator)), interval_(interval) {
    seek_to_first_sample();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const PromPP::Primitives::TimeInterval& interval() const noexcept { return interval_; }
  PROMPP_ALWAYS_INLINE void set_interval(const PromPP::Primitives::TimeInterval& interval) {
    interval_ = interval;
    counter_reset_ = false;
    sample_ = kInvalidSample;
    seek_to_first_sample();
  }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const { return sample_; }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const { return &sample_; }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const { return sample_.timestamp == kInvalidTimestamp; }

  PROMPP_ALWAYS_INLINE RateIterator& operator++() {
    seek_to_next_sample();
    return *this;
  }

  PROMPP_ALWAYS_INLINE RateIterator operator++(int) {
    const auto result = *this;
    ++*this;
    return result;
  }

 protected:
  static constexpr encoder::Sample kInvalidSample = {.timestamp = kInvalidTimestamp, .value = BareBones::Encoding::Gorilla::STALE_NAN};

  encoder::Sample sample_{kInvalidSample};
  Iterator iterator_;
  PromPP::Primitives::TimeInterval interval_;
  bool counter_reset_{};

  void seek_to_first_sample() {
    iterator_.template seek<SeekKind::kNext_Stop_NextAndStop>([this](PromPP::Primitives::Timestamp timestamp, double value) {
      if (timestamp < interval_.min) {
        return SeekResult::kNext;
      }
      if (timestamp > interval_.max) {
        return SeekResult::kStop;
      }

      if (BareBones::Encoding::Gorilla::isstalenan(value)) [[unlikely]] {
        return SeekResult::kNext;
      }

      sample_ = encoder::Sample{.timestamp = timestamp, .value = value};
      return SeekResult::kNextAndStop;
    });
  }

  PROMPP_ALWAYS_INLINE void seek_to_next_sample() {
    sample_.timestamp = kInvalidTimestamp;

    iterator_.template seek<SeekKind::kNext_Stop_NextAndStop>([this](PromPP::Primitives::Timestamp timestamp, double value) {
      if (counter_reset_) [[unlikely]] {
        counter_reset_ = false;
        sample_ = encoder::Sample{.timestamp = timestamp, .value = value};
        return SeekResult::kNextAndStop;
      }

      if (timestamp > interval_.max) {
        return SeekResult::kStop;
      }

      if (BareBones::Encoding::Gorilla::isstalenan(value)) [[unlikely]] {
        return SeekResult::kNext;
      }

      if (value < sample_.value) [[unlikely]] {
        counter_reset_ = true;
        if (sample_.timestamp != kInvalidTimestamp) {
          return SeekResult::kStop;
        }

        sample_ = encoder::Sample{.timestamp = timestamp, .value = value};
        return SeekResult::kNextAndStop;
      }

      sample_ = encoder::Sample{.timestamp = timestamp, .value = value};
      return SeekResult::kNext;
    });
  }
};

}  // namespace series_data::decoder::decorator