#pragma once

#include "primitives/primitives.h"
#include "series_data/decoder/universal_decode_iterator.h"

namespace series_data::decoder::decorator {

class RateIterator {
 public:
  DECODE_ITERATOR_TYPE_TRAITS();

  explicit RateIterator(PromPP::Primitives::TimeInterval interval) : interval_(interval) {}
  explicit RateIterator(UniversalDecodeIterator&& iterator, PromPP::Primitives::TimeInterval interval) : iterator_(iterator), interval_(interval) {
    seek_to_first_sample();
  }

  PROMPP_ALWAYS_INLINE RateIterator& operator=(UniversalDecodeIterator&& iterator) {
    iterator_ = std::move(iterator);
    counter_reset_ = false;
    seek_to_first_sample();
    return *this;
  }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const { return iterator_.operator*(); }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const { return iterator_.operator->(); }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const { return iterator_->timestamp == kInvalidTimestamp; }

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
  UniversalDecodeIterator iterator_;
  PromPP::Primitives::TimeInterval interval_;
  bool counter_reset_{};

  void seek_to_first_sample() {
    bool has_value{};

    iterator_.seek([&has_value, this](PromPP::Primitives::Timestamp timestamp, double value) {
      if (timestamp < interval_.min) {
        return SeekResult::kNext;
      }
      if (timestamp > interval_.max) {
        return SeekResult::kStop;
      }

      if (BareBones::Encoding::Gorilla::isstalenan(value)) [[unlikely]] {
        return SeekResult::kNext;
      }

      has_value = true;
      return SeekResult::kUpdateSampleNextAndStop;
    });

    if (!has_value) [[unlikely]] {
      iterator_.invalidate_sample();
    }
  }

  PROMPP_ALWAYS_INLINE void seek_to_next_sample() {
    bool has_value{};

    iterator_.seek([&has_value, this](PromPP::Primitives::Timestamp timestamp, double value) {
      if (counter_reset_) [[unlikely]] {
        counter_reset_ = false;
        has_value = true;
        return SeekResult::kUpdateSampleNextAndStop;
      }

      if (timestamp > interval_.max) {
        return SeekResult::kStop;
      }

      if (BareBones::Encoding::Gorilla::isstalenan(value)) [[unlikely]] {
        return SeekResult::kNext;
      }

      if (value < iterator_->value) [[unlikely]] {
        counter_reset_ = true;
        if (has_value) {
          return SeekResult::kStop;
        }

        has_value = true;
        return SeekResult::kUpdateSampleNextAndStop;
      }

      has_value = true;
      return SeekResult::kUpdateSample;
    });

    if (!has_value) [[unlikely]] {
      iterator_.invalidate_sample();
    }
  }
};

}  // namespace series_data::decoder::decorator