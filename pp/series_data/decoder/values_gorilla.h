#pragma once

#include "series_data/encoder/value/values_gorilla.h"
#include "traits.h"

namespace series_data::decoder {

class ValuesGorillaDecodeIterator : public SeparatedTimestampValueDecodeIteratorTrait {
 public:
  using Decoder = BareBones::Encoding::Gorilla::ValuesDecoder;

  template <class BitSequenceWithItemsCount>
  ValuesGorillaDecodeIterator(const BitSequenceWithItemsCount& timestamp_stream, const BareBones::BitSequenceReader& reader, bool is_last_stalenan)
      : ValuesGorillaDecodeIterator(timestamp_stream.count(), timestamp_stream.reader(), reader, is_last_stalenan) {}
  ValuesGorillaDecodeIterator(uint8_t samples_count,
                              const BareBones::BitSequenceReader& timestamp_reader,
                              const BareBones::BitSequenceReader& values_reader,
                              bool is_last_stalenan)
      : SeparatedTimestampValueDecodeIteratorTrait(samples_count, timestamp_reader, 0.0, is_last_stalenan), reader_(values_reader) {
    if (remaining_samples_ > 0) {
      decode_value<true>();
    }
  }

  PROMPP_ALWAYS_INLINE ValuesGorillaDecodeIterator& operator++() noexcept {
    if (decode_timestamp()) {
      decode_value<false>();
    }
    return *this;
  }

  PROMPP_ALWAYS_INLINE ValuesGorillaDecodeIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

  template <bool first>
  PROMPP_ALWAYS_INLINE static double decode_value(Decoder& decoder, BareBones::BitSequenceReader& reader) noexcept {
    if constexpr (first) {
      decoder.decode_first(reader);
    } else {
      decoder.decode(reader);
    }

    return decoder.value();
  }

 private:
  BareBones::BitSequenceReader reader_;
  Decoder decoder_;

  template <bool first>
  PROMPP_ALWAYS_INLINE void decode_value() noexcept {
    sample_.value = decode_value<first>(decoder_, reader_);
  }
};

}  // namespace series_data::decoder
