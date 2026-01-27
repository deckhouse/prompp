#pragma once

#include "series_data/decoder/traits.h"
#include "series_data/encoder/sample.h"

namespace series_data::decoder::decorator {

static constexpr PromPP::Primitives::Timestamp kNoDownsampling = 0;

template <class DecodeIterator>
class DownsamplingDecodeIterator {
 public:
  using Timestamp = PromPP::Primitives::Timestamp;

  enum class SampleType : uint8_t {
    kFirst = 0,
    kOther,
  };

  DECODE_ITERATOR_TYPE_TRAITS();

  explicit DownsamplingDecodeIterator(Timestamp interval) : DownsamplingDecodeIterator(DecodeIterator{}, interval) {}
  DownsamplingDecodeIterator(DecodeIterator&& iterator, Timestamp interval) : iterator_(std::move(iterator)), interval_(interval) {
    advance_to_next_sample<SampleType::kFirst>();
  }

  PROMPP_ALWAYS_INLINE DownsamplingDecodeIterator& operator=(DecodeIterator&& iterator) noexcept {
    iterator_ = std::move(iterator);
    advance_to_next_sample<SampleType::kFirst>();
    return *this;
  }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const noexcept { return iterator_.operator*(); }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const noexcept { return iterator_.operator->(); }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const noexcept { return (*this)->timestamp == kInvalidTimestamp; }

  PROMPP_ALWAYS_INLINE DownsamplingDecodeIterator& operator++() noexcept {
    advance_to_next_sample<SampleType::kOther>();
    return *this;
  }

  PROMPP_ALWAYS_INLINE DownsamplingDecodeIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

 private:
  DecodeIterator iterator_;
  Timestamp interval_;

  PROMPP_ALWAYS_INLINE static Timestamp round_up_to_step(Timestamp timestamp, Timestamp step) noexcept {
    const auto result = timestamp + step - 1;
    return result - result % step;
  }

  template <SampleType Type>
  PROMPP_ALWAYS_INLINE void advance_to_next_sample() noexcept {
    if (interval_ == kNoDownsampling) {
      if constexpr (Type == SampleType::kOther) {
        if (++iterator_ == DecodeIteratorSentinel{}) {
          iterator_.invalidate();
        }
      }
      return;
    }

    advance_to_last_sample_in_interval();
  }

  PROMPP_ALWAYS_INLINE void advance_to_last_sample_in_interval() noexcept {
    Timestamp sample_timestamp = kInvalidTimestamp;

    iterator_.seek([this, &sample_timestamp](Timestamp timestamp) noexcept {
      if (timestamp > sample_timestamp) {
        if (sample_timestamp != kInvalidTimestamp) [[likely]] {
          return SeekResult::kStop;
        }

        sample_timestamp = round_up_to_step(timestamp, interval_);
      }

      return SeekResult::kUpdateSample;
    });

    if (sample_timestamp == kInvalidTimestamp) [[unlikely]] {
      iterator_.invalidate();
    }
  }
};

}  // namespace series_data::decoder::decorator