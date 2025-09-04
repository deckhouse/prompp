#pragma once

#include <array>
#include <bit>
#include <cassert>
#include <climits>
#include <cstdint>

#include "preprocess.h"

#ifdef __x86_64__
#include <x86intrin.h>
#endif

namespace BareBones::Bit {

static_assert(std::endian::native == std::endian::little, "big endian arch is not yet supported");

inline __attribute__((always_inline)) uint32_t bextr(uint32_t src, uint32_t start, uint32_t len) noexcept {
#ifdef __BMI__
  return _bextr_u32(src, start, len);
#else
  assert(start < 32);
  assert(len <= 32);
  return (static_cast<uint64_t>(src) >> start) & ((static_cast<uint64_t>(1) << len) - 1);
#endif
}

inline __attribute__((always_inline)) uint64_t bextr(uint64_t src, uint32_t start, uint32_t len) noexcept {
#ifdef __BMI__
  return _bextr_u64(src, start, len);
#else
  assert(start < 64);
  assert(len <= 64);

  static constexpr auto lut = []() {
    std::array<uint64_t, 65> mask;

    for (uint8_t i = 0; i != 65; ++i) {
      mask[i] = ((static_cast<uint64_t>(i < 64) << (i & 63)) - 1u);
    }

    return mask;
  }();
  return (src >> start) & lut[len];
#endif
}

constexpr uint8_t kByteBits = CHAR_BIT;

template <class T>
PROMPP_ALWAYS_INLINE constexpr T to_bits(T bytes) noexcept {
  return bytes * kByteBits;
}

template <class T>
PROMPP_ALWAYS_INLINE constexpr T to_bytes(T bits) noexcept {
  return bits / kByteBits;
}

template <class T>
constexpr T to_ceil_bytes(T bits) noexcept {
  return (bits + kByteBits - 1) / kByteBits;
}

template <typename T>
constexpr std::size_t unit_bits = sizeof(T) * kByteBits;

template <typename Unit, typename Int>
constexpr Int to_units(Int bits) noexcept {
  return bits / unit_bits<Unit>;
}

template <typename Unit, typename Int>
constexpr Int to_ceil_units(Int bits) noexcept {
  return (bits + unit_bits<Unit> - 1) / unit_bits<Unit>;
}

constexpr uint8_t kUint64Bits = unit_bits<uint64_t>;

template <class T>
PROMPP_ALWAYS_INLINE constexpr T be(T value) noexcept {
  if constexpr (std::endian::native == std::endian::little) {
    return std::byteswap(value);
  } else {
    return value;
  }
}

template <class T>
PROMPP_ALWAYS_INLINE constexpr T be_to_native(T value) noexcept {
  if constexpr (std::endian::native == std::endian::little) {
    return std::byteswap(value);
  } else {
    return value;
  }
}

}  // namespace BareBones::Bit
