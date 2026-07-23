#pragma once

#include "primitives/primitives.h"
#include "series_data/decoder/universal_decode_iterator.h"

namespace series_data::decoder::decorator {

template <class Iterator = UniversalDecodeIterator>
class ResetsIterator {
 public:
  DECODE_ITERATOR_TYPE_TRAITS();

  explicit ResetsIterator(PromPP::Primitives::TimeInterval interval) : interval_(interval) {}
  explicit ResetsIterator(Iterator&& iterator, PromPP::Primitives::TimeInterval interval) : iterator_(std::move(iterator)), interval_(interval) {
    seek_to_first_sample();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const PromPP::Primitives::TimeInterval& interval() const noexcept { return interval_; }
  PROMPP_ALWAYS_INLINE void set_interval(const PromPP::Primitives::TimeInterval& interval) {
    interval_ = interval;
    seek_to_first_sample();
  }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const { return iterator_.operator*(); }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const { return iterator_.operator->(); }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const { return iterator_->timestamp == kInvalidTimestamp; }

  PROMPP_ALWAYS_INLINE ResetsIterator& operator++() {
    seek_to_next_sample();
    return *this;
  }

  PROMPP_ALWAYS_INLINE ResetsIterator operator++(int) {
    const auto result = *this;
    ++*this;
    return result;
  }

 protected:
  Iterator iterator_;
  PromPP::Primitives::TimeInterval interval_;

  void seek_to_first_sample() {
    bool has_value{};

    iterator_.template seek<SeekKind::kAll>([&has_value, this](PromPP::Primitives::Timestamp timestamp, double value) {
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
    bool has_resets{};
    double previous_value = iterator_->value;

    iterator_.template seek<SeekKind::kAll>([&has_resets, &previous_value, this](PromPP::Primitives::Timestamp timestamp, double value) {
      if (timestamp > interval_.max) {
        return SeekResult::kStop;
      }

      if (BareBones::Encoding::Gorilla::isstalenan(value)) [[unlikely]] {
        return SeekResult::kNext;
      }

      if (value < previous_value) [[unlikely]] {
        has_resets = true;
        previous_value = value;
        return SeekResult::kUpdateSampleNextAndStop;
      }

      previous_value = value;
      return SeekResult::kNext;
    });

    if (!has_resets) [[unlikely]] {
      iterator_.invalidate_sample();
    }
  }
};

}  // namespace series_data::decoder::decorator