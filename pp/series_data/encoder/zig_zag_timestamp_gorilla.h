#pragma once

#include "bare_bones/gorilla.h"

namespace series_data::encoder {

constexpr BareBones::Encoding::Gorilla::DodSignificantLengths kDodSignificantLengths = {.first = 4, .second = 12, .third = 21};

constexpr uint32_t kStaleNanMark = 0b011111;
constexpr uint32_t kStaleNanMarkBits = std::popcount(kStaleNanMark) + 1;
constexpr uint32_t kSwitchToValuesGorillaMark = 0b111111;
constexpr uint32_t kSwitchToValuesGorillaMarkBits = std::popcount(kSwitchToValuesGorillaMark);

enum class ValueType : uint8_t {
  kStaleNan = 0,
  kValue,
  kSwitchToValuesGorillaMark,
};

template <class DeltaType>
class PROMPP_ATTRIBUTE_PACKED ZigZagTimestampEncoder
    : public BareBones::Encoding::Gorilla::ZigZagTimestampEncoder<BareBones::Encoding::Gorilla::TimestampEncoderState<DeltaType>, kDodSignificantLengths> {
 public:
  template <class BitSequence>
  PROMPP_ALWAYS_INLINE void encode_delta_of_delta_with_stale_nan(double timestamp, BitSequence& stream) {
    if (BareBones::Encoding::Gorilla::isstalenan(timestamp)) [[unlikely]] {
      stream.push_back_bits_u32(kStaleNanMarkBits, kStaleNanMark);
    } else {
      const auto ts = static_cast<int64_t>(timestamp);
      const auto ts_delta = ts - this->state.last_ts;
      const int64_t delta_of_delta = ts_delta - this->state.last_ts_delta;
      const uint64_t ts_dod_zigzag = BareBones::Encoding::ZigZag::encode(delta_of_delta);

      encode_delta_of_delta_with_stale_nan<BitSequence>(ts_dod_zigzag, stream);

      this->state.last_ts_delta = ts_delta;
      this->state.last_ts = ts;
    }
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void encode_delta_of_delta_with_stale_nan(uint64_t ts_dod_zigzag, BitSequence& stream) {
    if (ts_dod_zigzag == 0) {
      stream.push_back_single_zero_bit();
    } else {
      const uint8_t ts_dod_significant_len = 64 - std::countl_zero(ts_dod_zigzag);

      if (ts_dod_significant_len <= kDodSignificantLengths.first) {
        stream.push_back_bits_u32(2 + kDodSignificantLengths.first, 0b01 | (ts_dod_zigzag << 2));
      } else if (ts_dod_significant_len <= kDodSignificantLengths.second) {
        stream.push_back_bits_u32(3 + kDodSignificantLengths.second, 0b011 | (ts_dod_zigzag << 3));
      } else if (ts_dod_significant_len <= kDodSignificantLengths.third) {
        stream.push_back_bits_u32(4 + kDodSignificantLengths.third, 0b0111 | (ts_dod_zigzag << 4));
      } else {
        stream.push_back_bits_u32(5, 0b01111);
        stream.push_back_u64_svbyte_2468(ts_dod_zigzag);
      }
    }
  }

  template <class BitSequence>
  PROMPP_ALWAYS_INLINE static void write_switch_to_values_gorilla_mark(BitSequence& stream) {
    stream.push_back_bits_u32(kSwitchToValuesGorillaMarkBits, kSwitchToValuesGorillaMark);
  }
};

class PROMPP_ATTRIBUTE_PACKED ZigZagTimestampDecoder : public BareBones::Encoding::Gorilla::ZigZagTimestampDecoder<kDodSignificantLengths> {
 public:
  using BareBones::Encoding::Gorilla::ZigZagTimestampDecoder<kDodSignificantLengths>::decode;

  PROMPP_ALWAYS_INLINE ValueType decode_delta_of_delta_with_stale_nan(BareBones::BitSequenceReader& reader) {
    if (const uint32_t buf = reader.read_u32(); buf & 0b1) {
      uint64_t dod_zigzag;

      if ((buf & 0b10) == 0) {
        dod_zigzag = BareBones::Bit::bextr(buf, 2, kDodSignificantLengths.first);
        reader.ff(2 + kDodSignificantLengths.first);
      } else if ((buf & 0b100) == 0) {
        dod_zigzag = BareBones::Bit::bextr(buf, 3, kDodSignificantLengths.second);
        reader.ff(3 + kDodSignificantLengths.second);
      } else if ((buf & 0b1000) == 0) {
        dod_zigzag = BareBones::Bit::bextr(buf, 4, kDodSignificantLengths.third);
        reader.ff(4 + kDodSignificantLengths.third);
      } else if ((buf & 0b10000) == 0) {
        reader.ff(5);
        dod_zigzag = reader.consume_u64_svbyte_2468();
      } else if ((buf & (kStaleNanMark + 1)) == 0) {
        reader.ff(kStaleNanMarkBits);
        return ValueType::kStaleNan;
      } else {
        reader.ff(kSwitchToValuesGorillaMarkBits);
        return ValueType::kSwitchToValuesGorillaMark;
      }

      state_.last_ts_delta += BareBones::Encoding::ZigZag::decode(dod_zigzag);
    } else {
      reader.ff(1);
    }

    state_.last_ts += state_.last_ts_delta;
    return ValueType::kValue;
  }

  ValueType decode(BareBones::BitSequenceReader& reader, BareBones::Encoding::Gorilla::GorillaState& state) noexcept {
    using enum BareBones::Encoding::Gorilla::GorillaState;

    if (state == kFirstPoint) [[unlikely]] {
      decode(reader);
      state = kSecondPoint;
    } else if (state == kSecondPoint) [[unlikely]] {
      decode_delta(reader);
      state = kOtherPoint;
    } else {
      return decode_delta_of_delta_with_stale_nan(reader);
    }

    return ValueType::kValue;
  }
};

}  // namespace series_data::encoder