#pragma once

#include "series_data/encoder/value/two_double_constant.h"
#include "traits.h"

namespace series_data::decoder {

class TwoDoubleConstantDecodeIterator : public SeparatedTimestampValueDecodeIteratorTrait<TwoDoubleConstantDecodeIterator> {
 public:
  template <class BitSequenceWithItemsCount>
  TwoDoubleConstantDecodeIterator(const BitSequenceWithItemsCount& timestamp_stream,
                                  const encoder::value::TwoDoubleConstantEncoder& encoder,
                                  bool is_last_stalenan)
      : SeparatedTimestampValueDecodeIteratorTrait(timestamp_stream, encoder.value1(), is_last_stalenan),
        value1_(encoder.value1()),
        value2_(encoder.value2()),
        value1_count_(encoder.value1_count()) {}

  TwoDoubleConstantDecodeIterator(uint8_t samples_count,
                                  const BareBones::BitSequenceReader& timestamp_reader,
                                  const encoder::value::TwoDoubleConstantEncoder& encoder,
                                  bool is_last_stalenan)
      : SeparatedTimestampValueDecodeIteratorTrait(samples_count, timestamp_reader, encoder.value1(), is_last_stalenan),
        value1_(encoder.value1()),
        value2_(encoder.value2()),
        value1_count_(encoder.value1_count()) {}

  PROMPP_ALWAYS_INLINE TwoDoubleConstantDecodeIterator& operator++() noexcept {
    if (decode()) [[likely]] {
      update_sample();
    }
    return *this;
  }

  PROMPP_ALWAYS_INLINE TwoDoubleConstantDecodeIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double decoded_value() const noexcept {
    if (remaining_samples_ == 1 && last_stalenan_) [[unlikely]] {
      return BareBones::Encoding::Gorilla::STALE_NAN;
    }

    return count_ <= value1_count_ ? value1_ : value2_;
  }

 private:
  friend Base;

  double value1_;
  double value2_;
  uint8_t value1_count_;
  uint8_t count_{1};

  PROMPP_ALWAYS_INLINE bool decode() noexcept {
    ++count_;
    return decode_timestamp();
  }

  PROMPP_ALWAYS_INLINE void update_sample() noexcept {
    sample_.timestamp = decoded_timestamp();
    sample_.value = decoded_value();
  }
};

}  // namespace series_data::decoder
