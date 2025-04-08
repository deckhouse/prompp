#pragma once

#include "series_data/encoder/value/asc_integer.h"
#include "traits.h"

namespace series_data::decoder {

class AscIntegerDecodeIterator : public SeparatedTimestampValueDecodeIteratorTrait {
 public:
  using Decoder = encoder::ZigZagTimestampDecoder;

  AscIntegerDecodeIterator(const encoder::BitSequenceWithItemsCount& timestamp_stream, const BareBones::BitSequenceReader& reader, bool is_last_stalenan)
      : AscIntegerDecodeIterator(timestamp_stream.count(), timestamp_stream.reader(), reader, is_last_stalenan) {}
  AscIntegerDecodeIterator(uint8_t samples_count,
                           const BareBones::BitSequenceReader& timestamp_reader,
                           const BareBones::BitSequenceReader& values_reader,
                           bool is_last_stalenan)
      : SeparatedTimestampValueDecodeIteratorTrait(samples_count, timestamp_reader, 0.0, is_last_stalenan), reader_(values_reader) {
    if (remaining_samples_ > 0) {
      decode_value();
    }
  }

  PROMPP_ALWAYS_INLINE AscIntegerDecodeIterator& operator++() noexcept {
    if (decode_timestamp()) {
      decode_value();
    }
    return *this;
  }

  PROMPP_ALWAYS_INLINE AscIntegerDecodeIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

 private:
  using GorillaState = BareBones::Encoding::Gorilla::GorillaState;

  Decoder decoder_;
  BareBones::BitSequenceReader reader_;
  BareBones::Encoding::Gorilla::GorillaState gorilla_state_{GorillaState::kFirstPoint};

  PROMPP_ALWAYS_INLINE void decode_value() noexcept { decoder_.decode(reader_, gorilla_state_, sample_.value); }
};

}  // namespace series_data::decoder
