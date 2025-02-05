#pragma once

#include "traits.h"

namespace series_data::decoder {

class ConstantDecodeIterator : public SeparatedTimestampValueDecodeIteratorTrait {
 public:
  ConstantDecodeIterator(const encoder::BitSequenceWithItemsCount& timestamp_stream, double value, bool is_last_stalenan)
      : SeparatedTimestampValueDecodeIteratorTrait(timestamp_stream, value, is_last_stalenan) {}
  ConstantDecodeIterator(uint8_t samples_count, const BareBones::BitSequenceReader& timestamp_reader, double value, bool is_last_stalenan)
      : SeparatedTimestampValueDecodeIteratorTrait(samples_count, timestamp_reader, value, is_last_stalenan) {}

  PROMPP_ALWAYS_INLINE ConstantDecodeIterator& operator++() noexcept {
    decode_timestamp();
    if (remaining_samples_ == 1 && last_stalenan_) [[unlikely]] {
      sample_.value = BareBones::Encoding::Gorilla::STALE_NAN;
    }
    return *this;
  }

  PROMPP_ALWAYS_INLINE ConstantDecodeIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }
};

}  // namespace series_data::decoder
