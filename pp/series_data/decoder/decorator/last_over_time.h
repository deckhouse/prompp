#pragma once

#include "primitives/primitives.h"
#include "series_data/decoder/universal_decode_iterator.h"

namespace series_data::decoder::decorator {

class LastOverTimeIterator {
 public:
  DECODE_ITERATOR_TYPE_TRAITS();

  explicit LastOverTimeIterator(PromPP::Primitives::TimeInterval interval) : interval_(interval) {}
  explicit LastOverTimeIterator(UniversalDecodeIterator&& iterator, PromPP::Primitives::TimeInterval interval) : iterator_(iterator), interval_(interval) {
    find_last_element();
  }

  PROMPP_ALWAYS_INLINE LastOverTimeIterator& operator=(UniversalDecodeIterator&& iterator) {
    iterator_ = std::move(iterator);
    find_last_element();
    return *this;
  }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const { return iterator_.operator*(); }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const { return iterator_.operator->(); }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const { return iterator_->timestamp == kInvalidTimestamp; }

  PROMPP_ALWAYS_INLINE LastOverTimeIterator& operator++() {
    iterator_.invalidate();
    return *this;
  }

  PROMPP_ALWAYS_INLINE LastOverTimeIterator operator++(int) {
    const auto result = *this;
    ++*this;
    return result;
  }

 private:
  UniversalDecodeIterator iterator_;
  PromPP::Primitives::TimeInterval interval_;

  PROMPP_ALWAYS_INLINE void find_last_element() {
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
      return SeekResult::kUpdateSample;
    });

    if (!has_value) [[unlikely]] {
      iterator_.invalidate();
    }
  }
};

}  // namespace series_data::decoder::decorator