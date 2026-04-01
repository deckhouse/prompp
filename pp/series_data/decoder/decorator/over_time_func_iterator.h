#pragma once

#include "primitives/primitives.h"
#include "series_data/decoder/universal_decode_iterator.h"

namespace series_data::decoder::decorator {

template <class SampleHandler>
class OverTimeFuncIterator {
 public:
  DECODE_ITERATOR_TYPE_TRAITS();

  explicit OverTimeFuncIterator(const PromPP::Primitives::TimeInterval& interval) : interval_(interval) {}
  explicit OverTimeFuncIterator(UniversalDecodeIterator&& iterator, PromPP::Primitives::TimeInterval interval) : iterator_(iterator), interval_(interval) {
    find_element();
  }

  PROMPP_ALWAYS_INLINE OverTimeFuncIterator& operator=(UniversalDecodeIterator&& iterator) {
    iterator_ = std::move(iterator);
    find_element();
    return *this;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const PromPP::Primitives::TimeInterval& interval() const noexcept { return interval_; }
  PROMPP_ALWAYS_INLINE void set_interval(const PromPP::Primitives::TimeInterval& interval) {
    interval_ = interval;
    find_element();
  }

  PROMPP_ALWAYS_INLINE void reset(UniversalDecodeIterator&& iterator, const PromPP::Primitives::TimeInterval& interval) {
    iterator_ = std::move(iterator);
    interval_ = interval;
    find_element();
  }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const { return iterator_.operator*(); }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const { return iterator_.operator->(); }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const { return iterator_->timestamp == kInvalidTimestamp; }

  PROMPP_ALWAYS_INLINE OverTimeFuncIterator& operator++() {
    iterator_.invalidate_sample();
    return *this;
  }

  PROMPP_ALWAYS_INLINE OverTimeFuncIterator operator++(int) {
    const auto result = *this;
    ++*this;
    return result;
  }

 protected:
  UniversalDecodeIterator iterator_;
  PromPP::Primitives::TimeInterval interval_;

  void find_element() {
    iterator_.seek_to(interval_.min);

    SampleHandler handler;
    iterator_.seek([&handler, this](PromPP::Primitives::Timestamp timestamp, double value) {
      if (timestamp > interval_.max) {
        return SeekResult::kStop;
      }

      if (BareBones::Encoding::Gorilla::isstalenan(value)) [[unlikely]] {
        return SeekResult::kNext;
      }

      return handler(timestamp, value);
    });

    handler.set_result(iterator_);
  }
};

}  // namespace series_data::decoder::decorator