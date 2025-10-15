#pragma once

#include <bit>
#include <cstddef>
#include <cstdint>

#include "bit"
#include "bit_sequence.h"
#include "preprocess.h"

namespace BareBones::Encoding {

class VarInt {
 public:
  template <std::integral T>
  static constexpr size_t kMaxVarIntLength = (Bit::to_bits(sizeof(T)) + 6) / 7;

  template <std::unsigned_integral T>
  PROMPP_ALWAYS_INLINE static size_t write(uint8_t* data, T value) noexcept {
    return write_internal(data, static_cast<uint64_t>(value));
  }

  template <std::signed_integral T>
  PROMPP_ALWAYS_INLINE static size_t write(uint8_t* data, T value) noexcept {
    return write_internal(data, static_cast<uint64_t>(unsignify(value)));
  }

  template <std::unsigned_integral T>
  PROMPP_ALWAYS_INLINE static T read(BitSequenceReader& reader) noexcept {
    T result = 0;
    uint8_t shift = 0;

    for (size_t i = 0; i < kMaxVarIntLength<T>; ++i) {
      const auto byte = static_cast<uint64_t>(reader.consume_bits_u32(Bit::kByteBits));
      if (byte < 0x80) {
        return result | static_cast<uint64_t>(byte) << shift;
      }

      result |= (byte & 0x7F) << shift;
      shift += 7;
    }

    return result;
  }

  template <std::signed_integral T>
  PROMPP_ALWAYS_INLINE static T read(BitSequenceReader& reader) noexcept {
    const auto value = read<std::make_unsigned_t<T>>(reader);
    return signify(value);
  }

  template <std::unsigned_integral T>
  PROMPP_ALWAYS_INLINE static T read(const uint8_t* data) noexcept {
    T result = 0;
    uint8_t shift = 0;

    for (size_t i = 0; i < kMaxVarIntLength<T>; ++i) {
      const auto byte = static_cast<uint64_t>(*data++);
      if (byte < 0x80) {
        return result | static_cast<uint64_t>(byte) << shift;
      }

      result |= (byte & 0x7F) << shift;
      shift += 7;
    }

    return result;
  }

  template <std::signed_integral T>
  PROMPP_ALWAYS_INLINE static T read(const uint8_t* data) noexcept {
    const auto value = read<std::make_unsigned_t<T>>(data);
    return signify(value);
  }

  template <std::unsigned_integral T>
  PROMPP_ALWAYS_INLINE static size_t length(T value) noexcept {
    if constexpr (sizeof(T) == 1) {
      return value < (1ULL << 7) ? 1 : 2;
    } else if constexpr (sizeof(T) == 2) {
      return value < (1ULL << 7) ? 1 : value < (1ULL << 14) ? 2 : 3;
    } else if constexpr (sizeof(T) == 4) {
      return value < (1ULL << 7) ? 1 : value < (1ULL << 14) ? 2 : value < (1ULL << 21) ? 3 : value < (1ULL << 28) ? 4 : 5;
    } else {
      return value == 0 ? 1 : (std::bit_width(value) + 6) / 7;
    }
  }

  template <std::signed_integral T>
  PROMPP_ALWAYS_INLINE static size_t length(T value) noexcept {
    return length(unsignify(value));
  }

 private:
  template <std::signed_integral T>
  PROMPP_ALWAYS_INLINE static auto unsignify(T value) noexcept {
    using U = std::make_unsigned_t<T>;
    auto unsigned_value = static_cast<U>(std::bit_cast<U>(value) << static_cast<U>(1));
    if (value < 0) {
      unsigned_value = ~unsigned_value;
    }

    return unsigned_value;
  }

  template <std::unsigned_integral T>
  PROMPP_ALWAYS_INLINE static auto signify(T value) noexcept {
    auto result = std::bit_cast<std::make_signed_t<T>>(static_cast<T>(value >> static_cast<T>(1)));
    if ((value & 1) != 0) {
      result = ~result;
    }
    return result;
  }

  PROMPP_ALWAYS_INLINE static size_t write_internal(uint8_t* data, uint64_t value) noexcept {
    auto p = data;
    while (value >= 128) {
      *p++ = 0x80 | (value & 0x7f);
      value >>= 7;
    }
    *p++ = static_cast<uint8_t>(value);
    return p - data;
  }
};

}  // namespace BareBones::Encoding