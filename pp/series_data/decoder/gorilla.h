#pragma once

#include "series_data/encoder/gorilla.h"
#include "traits.h"

namespace series_data::decoder {

template <std::unsigned_integral SampleCountType>
class GorillaDecodeIteratorGeneral : public DecodeIteratorTrait<SampleCountType> {
  using Base = DecodeIteratorTrait<SampleCountType>;

 public:
  explicit GorillaDecodeIteratorGeneral(const encoder::CompactBitSequence& stream, bool is_last_stalenan)
      : GorillaDecodeIteratorGeneral(encoder::BitSequenceWithItemsCount::count(stream), encoder::BitSequenceWithItemsCount::reader(stream), is_last_stalenan) {}
  GorillaDecodeIteratorGeneral(SampleCountType samples_count, const BareBones::BitSequenceReader& reader, bool is_last_stalenan)
      : Base(0.0, samples_count, is_last_stalenan), reader_(reader) {
    decode();
  }

  PROMPP_ALWAYS_INLINE GorillaDecodeIteratorGeneral& operator++() noexcept {
    --Base::remaining_samples_;
    decode();
    return *this;
  }

  PROMPP_ALWAYS_INLINE GorillaDecodeIteratorGeneral operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

 private:
  using Decoder = BareBones::Encoding::Gorilla::ValuesDecoder;

  BareBones::BitSequenceReader reader_;
  BareBones::Encoding::Gorilla::StreamDecoder<BareBones::Encoding::Gorilla::ZigZagTimestampDecoder<>, BareBones::Encoding::Gorilla::ValuesDecoder> decoder_;

  PROMPP_ALWAYS_INLINE void decode() noexcept {
    if (Base::remaining_samples_ > 0) {
      decoder_.decode(reader_, reader_);
      Base::sample_.value = decoder_.last_value();
      Base::sample_.timestamp = decoder_.last_timestamp();
    }
  }
};

using GorillaDecodeIterator = GorillaDecodeIteratorGeneral<uint8_t>;

}  // namespace series_data::decoder
