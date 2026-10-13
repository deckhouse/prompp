#pragma once

#include "bare_bones/bit_sequence.h"
#include "bare_bones/vector.h"
#include "state.h"

namespace series_data::encoder::timestamp {

class TimestampEncoder {
 public:
  template <class BitSequenceWithItemsCount>
  static void encode_first(TimestampEncoderCodec& encoder, int64_t timestamp, BitSequenceWithItemsCount& stream) {
    encoder.encode(timestamp, stream.stream);
  }

  template <class BitSequenceWithItemsCount>
  static void encode(TimestampEncoderCodec& encoder, int64_t timestamp, BitSequenceWithItemsCount& stream) {
    if (stream.inc_count() == 1) [[unlikely]] {
      encoder.encode_delta(timestamp, stream.stream);
    } else {
      encoder.encode_delta_of_delta(timestamp, stream.stream);
    }
  }
};

class TimestampDecoder {
 public:
  explicit constexpr TimestampDecoder(const BareBones::BitSequenceReader& reader) : reader_(reader) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t decode() noexcept {
    if (gorilla_state_ == GorillaState::kFirstPoint) [[unlikely]] {
      decoder_.decode(reader_);
      gorilla_state_ = GorillaState::kSecondPoint;
    } else if (gorilla_state_ == GorillaState::kSecondPoint) [[unlikely]] {
      decoder_.decode_delta(reader_);
      gorilla_state_ = GorillaState::kOtherPoint;
    } else {
      decoder_.decode_delta_of_delta(reader_);
    }

    return decoder_.timestamp();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool eof() const noexcept { return reader_.eof(); }

  [[nodiscard]] static int64_t decode_first(BareBones::BitSequenceReader reader) noexcept {
    TimestampDecoderCodec decoder;
    decoder.decode(reader);
    return decoder.timestamp();
  }

  [[nodiscard]] static BareBones::Vector<int64_t> decode_all(const BareBones::BitSequenceReader& reader, uint8_t count) noexcept {
    BareBones::Vector<int64_t> values;

    TimestampDecoder decoder(reader);
    for (uint8_t i = 0; i < count; ++i) {
      values.emplace_back(decoder.decode());
    }

    return values;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t timestamp() const noexcept { return decoder_.timestamp(); }

 private:
  using GorillaState = BareBones::Encoding::Gorilla::GorillaState;

  BareBones::BitSequenceReader reader_;
  TimestampDecoderCodec decoder_;
  GorillaState gorilla_state_{GorillaState::kFirstPoint};
};

}  // namespace series_data::encoder::timestamp
