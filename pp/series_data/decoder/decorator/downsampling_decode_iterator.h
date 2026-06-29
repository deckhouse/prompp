#pragma once

#include "series_data/decoder/traits.h"
#include "series_data/encoder/sample.h"

namespace series_data::decoder::decorator {

static constexpr PromPP::Primitives::Timestamp kNoDownsampling = 0;

template <class DecodeIterator>
class DownsamplingDecodeIterator {
 public:
  using Timestamp = PromPP::Primitives::Timestamp;

  DECODE_ITERATOR_TYPE_TRAITS();

  explicit DownsamplingDecodeIterator(Timestamp interval) : DownsamplingDecodeIterator(DecodeIterator{}, interval) {}
  DownsamplingDecodeIterator(DecodeIterator&& iterator, Timestamp interval) : iterator_(std::move(iterator)), interval_(interval) {
    advance_to_last_sample_in_interval();
  }

  PROMPP_ALWAYS_INLINE DownsamplingDecodeIterator& operator=(DecodeIterator&& iterator) noexcept {
    iterator_ = std::move(iterator);
    advance_to_last_sample_in_interval();
    return *this;
  }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const noexcept { return iterator_.operator*(); }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const noexcept { return iterator_.operator->(); }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const noexcept { return (*this)->timestamp == kInvalidTimestamp; }

  PROMPP_ALWAYS_INLINE DownsamplingDecodeIterator& operator++() noexcept {
    advance_to_last_sample_in_interval();
    return *this;
  }

  PROMPP_ALWAYS_INLINE DownsamplingDecodeIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

  template <SeekKind, class SeekHandler>
  PROMPP_ALWAYS_INLINE void seek(SeekHandler&& handler) {
    for (; *this != DecodeIteratorSentinel{}; ++*this) {
      if (handler(iterator_->timestamp, iterator_->value) == SeekResult::kStop) [[unlikely]] {
        return;
      }
    }
  }

 private:
  DecodeIterator iterator_;
  Timestamp interval_;

  PROMPP_ALWAYS_INLINE static Timestamp round_up_to_step(Timestamp timestamp, Timestamp step) noexcept {
    const auto result = timestamp + step - 1;
    return result - result % step;
  }

  PROMPP_ALWAYS_INLINE void advance_to_last_sample_in_interval() noexcept {
    Timestamp sample_timestamp = kInvalidTimestamp;

    iterator_.template seek<SeekKind::kUpdateSample_Stop>([this, &sample_timestamp](Timestamp timestamp) noexcept {
      if (timestamp > sample_timestamp) {
        if (sample_timestamp != kInvalidTimestamp) [[likely]] {
          return SeekResult::kStop;
        }

        sample_timestamp = round_up_to_step(timestamp, interval_);
      }

      return SeekResult::kUpdateSample;
    });

    if (sample_timestamp == kInvalidTimestamp) [[unlikely]] {
      iterator_.invalidate_sample();
    }
  }
};

}  // namespace series_data::decoder::decorator