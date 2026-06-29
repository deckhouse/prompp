#pragma once

#include "primitives/primitives.h"
#include "series_data/decoder/universal_decode_iterator.h"

namespace series_data::decoder::decorator {

template <class Iterator = UniversalDecodeIterator>
class IRateIterator {
 public:
  DECODE_ITERATOR_TYPE_TRAITS();

  explicit IRateIterator(PromPP::Primitives::TimeInterval interval) : interval_(interval) {}
  explicit IRateIterator(Iterator&& iterator, PromPP::Primitives::TimeInterval interval) : iterator_(std::move(iterator)), interval_(interval) {
    find_last_2samples();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const PromPP::Primitives::TimeInterval& interval() const noexcept { return interval_; }
  PROMPP_ALWAYS_INLINE void set_interval(const PromPP::Primitives::TimeInterval& interval) {
    interval_ = interval;
    find_last_2samples();
  }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const { return sample_; }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const { return &sample_; }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const { return sample_.timestamp == kInvalidTimestamp; }

  PROMPP_ALWAYS_INLINE IRateIterator& operator++() {
    sample_ = *iterator_;
    iterator_.invalidate_sample();
    return *this;
  }

  PROMPP_ALWAYS_INLINE IRateIterator operator++(int) {
    const auto result = *this;
    ++*this;
    return result;
  }

 protected:
  static constexpr encoder::Sample kInvalidSample = {.timestamp = kInvalidTimestamp, .value = BareBones::Encoding::Gorilla::STALE_NAN};

  encoder::Sample sample_{kInvalidSample};
  Iterator iterator_;
  PromPP::Primitives::TimeInterval interval_;

  void find_last_2samples() {
    sample_ = kInvalidSample;

    iterator_.template seek<SeekKind::kUpdateSample_Next_Stop>([this](PromPP::Primitives::Timestamp timestamp, double value) {
      if (timestamp < interval_.min) {
        return SeekResult::kNext;
      }
      if (timestamp > interval_.max) {
        return SeekResult::kStop;
      }

      if (BareBones::Encoding::Gorilla::isstalenan(value)) [[unlikely]] {
        return SeekResult::kNext;
      }

      sample_ = *iterator_;
      return SeekResult::kUpdateSample;
    });

    if (sample_.timestamp == iterator_->timestamp) {
      iterator_.invalidate_sample();
    }
  }
};

}  // namespace series_data::decoder::decorator