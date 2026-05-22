#pragma once

#include "primitives/primitives.h"
#include "series_data/decoder/universal_decode_iterator.h"

namespace series_data::decoder::decorator {

template <class Iterator = UniversalDecodeIterator>
class ChangesIterator {
 public:
  DECODE_ITERATOR_TYPE_TRAITS();

  explicit ChangesIterator(PromPP::Primitives::TimeInterval interval) : interval_(interval) {}
  explicit ChangesIterator(Iterator&& iterator, PromPP::Primitives::TimeInterval interval) : iterator_(std::move(iterator)), interval_(interval) {
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

  PROMPP_ALWAYS_INLINE ChangesIterator& operator++() {
    seek_to_next_sample();
    return *this;
  }

  PROMPP_ALWAYS_INLINE ChangesIterator operator++(int) {
    const auto result = *this;
    ++*this;
    return result;
  }

 protected:
  Iterator iterator_;
  PromPP::Primitives::TimeInterval interval_;

  void seek_to_first_sample() {
    bool has_value{};

    iterator_.template seek<SeekKind::kAll>([&has_value, this](PromPP::Primitives::Timestamp timestamp) {
      if (timestamp < interval_.min) {
        return SeekResult::kNext;
      }
      if (timestamp > interval_.max) {
        return SeekResult::kStop;
      }

      has_value = true;
      return SeekResult::kUpdateSampleNextAndStop;
    });

    if (!has_value) [[unlikely]] {
      iterator_.invalidate_sample();
    }
  }

  PROMPP_ALWAYS_INLINE void seek_to_next_sample() {
    bool has_changes{};

    iterator_.template seek<SeekKind::kAll>([&has_changes, this](PromPP::Primitives::Timestamp timestamp, double value) {
      using BareBones::Encoding::Gorilla::isstalenan;

      if (timestamp > interval_.max) {
        return SeekResult::kStop;
      }

      if (value != iterator_->value) {
        has_changes = true;
        return SeekResult::kUpdateSampleNextAndStop;
      }

      return SeekResult::kNext;
    });

    if (!has_changes) [[unlikely]] {
      iterator_.invalidate_sample();
    }
  }
};

}  // namespace series_data::decoder::decorator