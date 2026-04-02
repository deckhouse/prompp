#pragma once

#include "primitives/primitives.h"
#include "series_data/decoder/universal_decode_iterator.h"

namespace series_data::decoder::decorator {

class IRateIterator {
 public:
  DECODE_ITERATOR_TYPE_TRAITS();

  explicit IRateIterator(PromPP::Primitives::TimeInterval interval) : interval_(interval) {}
  explicit IRateIterator(UniversalDecodeIterator&& iterator, PromPP::Primitives::TimeInterval interval) : iterator_(iterator), interval_(interval) {
    find_last_2samples();
  }

  PROMPP_ALWAYS_INLINE IRateIterator& operator=(UniversalDecodeIterator&& iterator) {
    iterator_ = std::move(iterator);
    sample_ = encoder::Sample{.timestamp = kInvalidTimestamp};
    find_last_2samples();
    return *this;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const PromPP::Primitives::TimeInterval& interval() const noexcept { return interval_; }
  PROMPP_ALWAYS_INLINE void set_interval(const PromPP::Primitives::TimeInterval& interval) {
    interval_ = interval;
    sample_ = encoder::Sample{.timestamp = kInvalidTimestamp};
    find_last_2samples();
  }

  PROMPP_ALWAYS_INLINE void reset(UniversalDecodeIterator&& iterator, const PromPP::Primitives::TimeInterval& interval) {
    iterator_ = std::move(iterator);
    interval_ = interval;
    sample_ = encoder::Sample{.timestamp = kInvalidTimestamp};
    find_last_2samples();
  }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const { return iterator_.operator*(); }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const { return iterator_.operator->(); }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const { return iterator_->timestamp == kInvalidTimestamp; }

  PROMPP_ALWAYS_INLINE IRateIterator& operator++() {
    iterator_.set(sample_);
    sample_.timestamp = kInvalidTimestamp;
    return *this;
  }

  PROMPP_ALWAYS_INLINE IRateIterator operator++(int) {
    const auto result = *this;
    ++*this;
    return result;
  }

 protected:
  UniversalDecodeIterator iterator_;
  PromPP::Primitives::TimeInterval interval_;
  encoder::Sample sample_{.timestamp = kInvalidTimestamp};

  void find_last_2samples() {
    iterator_.seek([this](PromPP::Primitives::Timestamp timestamp, double value) {
      if (timestamp < interval_.min) {
        return SeekResult::kNext;
      }
      if (timestamp > interval_.max) {
        return SeekResult::kStop;
      }

      if (BareBones::Encoding::Gorilla::isstalenan(value)) [[unlikely]] {
        return SeekResult::kNext;
      }

      iterator_.set(sample_);
      sample_.timestamp = timestamp;
      sample_.value = value;
      return SeekResult::kNext;
    });

    if (sample_.timestamp == kInvalidTimestamp) [[unlikely]] {
      iterator_.invalidate_sample();
    }
  }
};

}  // namespace series_data::decoder::decorator