#pragma once

#include "series_data/encoder/value/two_double_constant.h"
#include "traits.h"

namespace series_data::decoder {

class TwoDoubleConstantDecodeIterator : public SeparatedTimestampValueDecodeIteratorTrait {
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
    if (decode_timestamp()) {
      ++count_;

      if (remaining_samples_ == 1 && last_stalenan_) [[unlikely]] {
        sample_.value = BareBones::Encoding::Gorilla::STALE_NAN;
      } else [[likely]] {
        sample_.value = count_ <= value1_count_ ? value1_ : value2_;
      }
    }
    return *this;
  }

  PROMPP_ALWAYS_INLINE TwoDoubleConstantDecodeIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

 private:
  double value1_;
  double value2_;
  uint8_t value1_count_;
  uint8_t count_{1};
};

}  // namespace series_data::decoder
