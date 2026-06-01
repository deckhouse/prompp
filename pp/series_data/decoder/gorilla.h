#pragma once

#include "series_data/encoder/gorilla.h"
#include "traits.h"

namespace series_data::decoder {

template <std::unsigned_integral SampleCountType>
class GorillaDecodeIteratorGeneral : public DecodeIteratorTrait<GorillaDecodeIteratorGeneral<SampleCountType>> {
  using Base = DecodeIteratorTrait<GorillaDecodeIteratorGeneral>;

 public:
  template <class CompactBitSequence>
  explicit GorillaDecodeIteratorGeneral(const CompactBitSequence& stream, bool is_last_stalenan)
      : GorillaDecodeIteratorGeneral(encoder::bit_sequence_items_count(stream.raw_bytes()), encoder::bit_sequence_reader(stream.bytes()), is_last_stalenan) {}
  GorillaDecodeIteratorGeneral(SampleCountType samples_count, const BareBones::BitSequenceReader& reader, bool is_last_stalenan)
      : data_{.remaining_samples = samples_count, .last_stalenan = is_last_stalenan}, reader_(reader) {
    if (data_.remaining_samples > 0) [[likely]] {
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

  [[nodiscard]] PROMPP_ALWAYS_INLINE double decoded_value() const noexcept { return decoder_.last_value(); }

 protected:
  friend Base;

  using Decoder = BareBones::Encoding::Gorilla::ValuesDecoder;

  DefaultDecodeIteratorData<SampleCountType> data_;
  BareBones::BitSequenceReader reader_;
  BareBones::Encoding::Gorilla::StreamDecoder<BareBones::Encoding::Gorilla::ZigZagTimestampDecoder<>, BareBones::Encoding::Gorilla::ValuesDecoder> decoder_;

 private:
  [[nodiscard]] PROMPP_ALWAYS_INLINE PromPP::Primitives::Timestamp decoded_timestamp() const noexcept { return decoder_.last_timestamp(); }

  PROMPP_ALWAYS_INLINE bool decode() noexcept {
    if (--data_.remaining_samples > 0) [[likely]] {
      decoder_.decode(reader_, reader_);
      return true;
    }

    return false;
  }

  PROMPP_ALWAYS_INLINE void update_sample() noexcept {
    data_.sample.value = decoder_.last_value();
    data_.sample.timestamp = decoder_.last_timestamp();
  }
};

using GorillaDecodeIterator = GorillaDecodeIteratorGeneral<uint8_t>;

}  // namespace series_data::decoder
