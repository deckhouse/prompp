#pragma once

#include <cstdint>

#include "preprocess.h"

namespace BareBones::Encoding::ZigZag {

PROMPP(inline) uint64_t encode(int64_t val) noexcept {
  // cppcheck-suppress shiftTooManyBitsSigned
  return (static_cast<uint64_t>(val) + static_cast<uint64_t>(val)) ^ (val >> 63);
}

PROMPP(inline) uint32_t encode(int32_t val) noexcept {
  // cppcheck-suppress shiftTooManyBitsSigned
  return (static_cast<uint32_t>(val) + static_cast<uint32_t>(val)) ^ (val >> 31);
}

PROMPP(inline) int64_t decode(uint64_t val) noexcept { return (val >> 1) ^ (0 - (val & 1)); }

PROMPP(inline) int32_t decode(uint32_t val) noexcept { return (val >> 1) ^ (0 - (val & 1)); }

}  // namespace BareBones::Encoding::ZigZag