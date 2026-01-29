#pragma once

#include "primitives/primitives.h"
#include "series_data/decoder/universal_decode_iterator.h"

namespace series_data::decoder::decorator {

class MaxOverTimeIterator {
 public:
  DECODE_ITERATOR_TYPE_TRAITS();

  explicit MaxOverTimeIterator(PromPP::Primitives::TimeInterval interval) : interval_(interval) {}
  explicit MaxOverTimeIterator(UniversalDecodeIterator&& iterator, PromPP::Primitives::TimeInterval interval) : iterator_(iterator), interval_(interval) {
    find_max_element();
  }

  PROMPP_ALWAYS_INLINE MaxOverTimeIterator& operator=(UniversalDecodeIterator&& iterator) {
    iterator_ = std::move(iterator);
    find_max_element();
    return *this;
  }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const { return iterator_.operator*(); }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const { return iterator_.operator->(); }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const { return iterator_->timestamp == kInvalidTimestamp; }

  PROMPP_ALWAYS_INLINE MaxOverTimeIterator& operator++() {
    iterator_.invalidate();
    return *this;
  }

  PROMPP_ALWAYS_INLINE MaxOverTimeIterator operator++(int) {
    const auto result = *this;
    ++*this;
    return result;
  }

 private:
  UniversalDecodeIterator iterator_;
  PromPP::Primitives::TimeInterval interval_;

  PROMPP_ALWAYS_INLINE void find_max_element() {
    double max_value = BareBones::Encoding::Gorilla::STALE_NAN;

    iterator_.seek([&max_value, this](PromPP::Primitives::Timestamp timestamp, double value) {
      if (timestamp < interval_.min) {
        return SeekResult::kNext;
      }
      if (timestamp > interval_.max) {
        return SeekResult::kStop;
      }

      if (BareBones::Encoding::Gorilla::isstalenan(value)) [[unlikely]] {
        return SeekResult::kNext;
      }

      if (BareBones::Encoding::Gorilla::isstalenan(max_value) || value > max_value) {
        max_value = value;
        return SeekResult::kUpdateSample;
      }

      return SeekResult::kNext;
    });

    if (BareBones::Encoding::Gorilla::isstalenan(max_value)) [[unlikely]] {
      iterator_.invalidate();
    }
  }
};

}  // namespace series_data::decoder::decorator