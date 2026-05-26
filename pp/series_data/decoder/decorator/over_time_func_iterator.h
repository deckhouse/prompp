#pragma once

#include "primitives/primitives.h"
#include "series_data/decoder/universal_decode_iterator.h"

namespace series_data::decoder::decorator {

template <class SampleHandler, class Iterator, bool SkipStaleNans>
class OverTimeFuncIterator {
 public:
  DECODE_ITERATOR_TYPE_TRAITS();

  explicit OverTimeFuncIterator(const PromPP::Primitives::TimeInterval& interval) : interval_(interval) {}
  explicit OverTimeFuncIterator(Iterator&& iterator, PromPP::Primitives::TimeInterval interval) : iterator_(std::move(iterator)), interval_(interval) {
    find_element();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const PromPP::Primitives::TimeInterval& interval() const noexcept { return interval_; }
  PROMPP_ALWAYS_INLINE void set_interval(const PromPP::Primitives::TimeInterval& interval) {
    interval_ = interval;
    find_element();
  }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const { return sample_; }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const { return &sample_; }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const { return sample_.timestamp == kInvalidTimestamp; }

  PROMPP_ALWAYS_INLINE OverTimeFuncIterator& operator++() {
    sample_.timestamp = kInvalidTimestamp;
    return *this;
  }

  PROMPP_ALWAYS_INLINE OverTimeFuncIterator operator++(int) {
    const auto result = *this;
    ++*this;
    return result;
  }

 protected:
  encoder::Sample sample_;
  Iterator iterator_;
  PromPP::Primitives::TimeInterval interval_;

  void find_element() {
    sample_ = encoder::Sample{.timestamp = kInvalidTimestamp, .value = BareBones::Encoding::Gorilla::STALE_NAN};
    SampleHandler handler{sample_, interval_};

    iterator_.template seek<SeekKind::kNext_Stop>([&handler, this](PromPP::Primitives::Timestamp timestamp, double value) {
      if (timestamp < interval_.min) [[unlikely]] {
        return SeekResult::kNext;
      }

      if (timestamp > interval_.max) [[unlikely]] {
        return SeekResult::kStop;
      }

      if (SkipStaleNans && BareBones::Encoding::Gorilla::isstalenan(value)) [[unlikely]] {
        return SeekResult::kNext;
      }

      handler(timestamp, value);
      return SeekResult::kNext;
    });
  }
};

}  // namespace series_data::decoder::decorator