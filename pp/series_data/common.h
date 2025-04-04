#pragma once

#include <cstdint>
#include <ranges>

#include "bare_bones/algorithm.h"

namespace series_data {

enum class EncodingType : uint8_t {
  kUnknown,
  kUint32Constant,
  kFloat32Constant,
  kDoubleConstant,
  kTwoDoubleConstant,
  kAscInteger,
  kValuesGorilla,
  kGorilla,
};

inline auto GetEncodingTypeRange() {
  return std::ranges::views::iota(static_cast<std::underlying_type_t<EncodingType>>(EncodingType::kUint32Constant),
                                  static_cast<std::underlying_type_t<EncodingType>>(static_cast<uint64_t>(EncodingType::kGorilla) + 1)) |
         std::ranges::views::transform([](std::underlying_type_t<EncodingType> val) { return static_cast<EncodingType>(val); });
}

struct EncodingState {
  EncodingType encoding_type : 7;
  bool has_last_stalenan : 1;

  bool operator==(const EncodingState&) const noexcept = default;
};

constexpr PROMPP_ALWAYS_INLINE bool is_constant_encoder(EncodingType encoding_type) noexcept {
  using enum EncodingType;
  return BareBones::is_in(encoding_type, kUint32Constant, kFloat32Constant, kDoubleConstant, kTwoDoubleConstant);
}

constexpr PROMPP_ALWAYS_INLINE bool is_gorilla_based_encoder(EncodingType encoding_type) noexcept {
  using enum EncodingType;
  return BareBones::is_in(encoding_type, kAscInteger, kValuesGorilla, kGorilla);
}

constexpr PROMPP_ALWAYS_INLINE bool is_variant_encoder(EncodingType encoding_type) noexcept {
  using enum EncodingType;
  return BareBones::is_in(encoding_type, kDoubleConstant, kTwoDoubleConstant, kAscInteger, kValuesGorilla);
}

}  // namespace series_data