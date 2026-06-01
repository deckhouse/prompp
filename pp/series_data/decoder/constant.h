#pragma once

#include "traits.h"

namespace series_data::decoder {

class ConstantDecodeIterator : public SeparatedTimestampValueDecodeIteratorTrait<ConstantDecodeIterator> {
 public:
  template <class BitSequenceWithItemsCount>
  ConstantDecodeIterator(const BitSequenceWithItemsCount& timestamp_stream, double value, bool is_last_stalenan)
      : SeparatedTimestampValueDecodeIteratorTrait(timestamp_stream, value, is_last_stalenan) {}
  constexpr ConstantDecodeIterator(uint8_t samples_count, const BareBones::BitSequenceReader& timestamp_reader, double value, bool is_last_stalenan)
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
    if (data_.remaining_samples == 1 && data_.last_stalenan) [[unlikely]] {
      return BareBones::Encoding::Gorilla::STALE_NAN;
    }

    return data_.sample.value;
  }

 protected:
  friend Base;

  PROMPP_ALWAYS_INLINE bool decode() noexcept { return decode_timestamp(); }

  PROMPP_ALWAYS_INLINE void update_sample() noexcept {
    data_.sample.timestamp = decoded_timestamp();
    data_.sample.value = decoded_value();
  }
};

}  // namespace series_data::decoder
