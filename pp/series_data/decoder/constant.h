#pragma once

#include "traits.h"

namespace series_data::decoder {

class ConstantDecodeIterator : public SeparatedTimestampValueDecodeIteratorTrait<ConstantDecodeIterator> {
 public:
  ConstantDecodeIterator(const encoder::BitSequenceWithItemsCount& timestamp_stream, double value, bool is_last_stalenan)
      : SeparatedTimestampValueDecodeIteratorTrait(timestamp_stream, value, is_last_stalenan) {}
  ConstantDecodeIterator(uint8_t samples_count, const BareBones::BitSequenceReader& timestamp_reader, double value, bool is_last_stalenan)
      : SeparatedTimestampValueDecodeIteratorTrait(samples_count, timestamp_reader, value, is_last_stalenan) {}

  PROMPP_ALWAYS_INLINE ConstantDecodeIterator& operator++() noexcept {
    decode_timestamp();
    update_sample();
    return *this;
  }

  PROMPP_ALWAYS_INLINE ConstantDecodeIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double decoded_value() const noexcept {
    if (remaining_samples_ == 1 && last_stalenan_) [[unlikely]] {
      return BareBones::Encoding::Gorilla::STALE_NAN;
    }

    return sample_.value;
  }

 protected:
  friend Base;

  PROMPP_ALWAYS_INLINE bool decode() noexcept { return decode_timestamp(); }

  PROMPP_ALWAYS_INLINE void update_sample() noexcept {
    sample_.timestamp = decoded_timestamp();
    sample_.value = decoded_value();
  }
};

}  // namespace series_data::decoder
