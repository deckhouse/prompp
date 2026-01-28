#pragma once

#include "series_data/encoder/gorilla.h"
#include "traits.h"

namespace series_data::decoder {

template <std::unsigned_integral SampleCountType>
class GorillaDecodeIteratorGeneral : public DecodeIteratorTrait<GorillaDecodeIteratorGeneral<SampleCountType>, SampleCountType> {
  using Base = DecodeIteratorTrait<GorillaDecodeIteratorGeneral, SampleCountType>;

 public:
  explicit GorillaDecodeIteratorGeneral(const encoder::CompactBitSequence& stream, bool is_last_stalenan)
      : GorillaDecodeIteratorGeneral(encoder::BitSequenceWithItemsCount::count(stream), encoder::BitSequenceWithItemsCount::reader(stream), is_last_stalenan) {}
  GorillaDecodeIteratorGeneral(SampleCountType samples_count, const BareBones::BitSequenceReader& reader, bool is_last_stalenan)
      : Base(0.0, samples_count, is_last_stalenan), reader_(reader) {
    if (Base::remaining_samples_ > 0) [[likely]] {
      decoder_.decode(reader_, reader_);
      update_sample();
    }
  }

  PROMPP_ALWAYS_INLINE GorillaDecodeIteratorGeneral& operator++() noexcept {
    if (decode()) [[likely]] {
      update_sample();
    }
    return *this;
  }

  PROMPP_ALWAYS_INLINE GorillaDecodeIteratorGeneral operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double decoded_value() const noexcept {
    return decoder_.last_value();
  }

 private:
  friend Base;

  using Decoder = BareBones::Encoding::Gorilla::ValuesDecoder;

  BareBones::BitSequenceReader reader_;
  BareBones::Encoding::Gorilla::StreamDecoder<BareBones::Encoding::Gorilla::ZigZagTimestampDecoder<>, BareBones::Encoding::Gorilla::ValuesDecoder> decoder_;

  [[nodiscard]] PROMPP_ALWAYS_INLINE PromPP::Primitives::Timestamp decoded_timestamp() const noexcept { return decoder_.last_timestamp(); }

  PROMPP_ALWAYS_INLINE bool decode() noexcept {
    if (--Base::remaining_samples_ > 0) [[likely]] {
      decoder_.decode(reader_, reader_);
      return true;
    }

    return false;
  }

  PROMPP_ALWAYS_INLINE void update_sample() noexcept {
    Base::sample_.value = decoder_.last_value();
    Base::sample_.timestamp = decoder_.last_timestamp();
  }
};

using GorillaDecodeIterator = GorillaDecodeIteratorGeneral<uint8_t>;

}  // namespace series_data::decoder
