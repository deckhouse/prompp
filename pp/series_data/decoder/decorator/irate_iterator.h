#pragma once

#include "primitives/primitives.h"
#include "series_data/decoder/universal_decode_iterator.h"

namespace series_data::decoder::decorator {

class IRateIterator {
 public:
  DECODE_ITERATOR_TYPE_TRAITS();

  explicit IRateIterator(PromPP::Primitives::TimeInterval interval) : interval_(interval) { reset_samples(); }
  explicit IRateIterator(UniversalDecodeIterator&& iterator, PromPP::Primitives::TimeInterval interval) : iterator_(iterator), interval_(interval) {
    reset_samples();
    find_last_2samples();
  }

  PROMPP_ALWAYS_INLINE IRateIterator& operator=(UniversalDecodeIterator&& iterator) {
    iterator_ = std::move(iterator);
    reset_samples();
    find_last_2samples();
    return *this;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const PromPP::Primitives::TimeInterval& interval() const noexcept { return interval_; }
  PROMPP_ALWAYS_INLINE void set_interval(const PromPP::Primitives::TimeInterval& interval) {
    interval_ = interval;
    reset_samples();
    find_last_2samples();
  }

  PROMPP_ALWAYS_INLINE void reset(UniversalDecodeIterator&& iterator, const PromPP::Primitives::TimeInterval& interval) {
    iterator_ = std::move(iterator);
    interval_ = interval;
    reset_samples();
    find_last_2samples();
  }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const { return samples_[0]; }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const { return &samples_[0]; }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const { return samples_[0].timestamp == kInvalidTimestamp; }

  PROMPP_ALWAYS_INLINE IRateIterator& operator++() {
    samples_[0] = samples_[1];
    samples_[1].timestamp = kInvalidTimestamp;
    return *this;
  }

  PROMPP_ALWAYS_INLINE IRateIterator operator++(int) {
    const auto result = *this;
    ++*this;
    return result;
  }

 protected:
  std::array<encoder::Sample, 2> samples_;
  UniversalDecodeIterator iterator_;
  PromPP::Primitives::TimeInterval interval_;

  PROMPP_ALWAYS_INLINE void reset_samples() {
    samples_[0] = samples_[1] = encoder::Sample{.timestamp = kInvalidTimestamp, .value = BareBones::Encoding::Gorilla::STALE_NAN};
  }

  void find_last_2samples() {
    iterator_.seek<SeekKind::kNext_Stop>([this](PromPP::Primitives::Timestamp timestamp, double value) {
      if (timestamp < interval_.min) {
        return SeekResult::kNext;
      }
      if (timestamp > interval_.max) {
        return SeekResult::kStop;
      }

      if (BareBones::Encoding::Gorilla::isstalenan(value)) [[unlikely]] {
        return SeekResult::kNext;
      }

      samples_[0] = samples_[1];
      samples_[1] = encoder::Sample{.timestamp = timestamp, .value = value};
      return SeekResult::kNext;
    });

    if (samples_[1].timestamp == kInvalidTimestamp) [[unlikely]] {
      iterator_.invalidate_sample();
    }
  }
};

}  // namespace series_data::decoder::decorator