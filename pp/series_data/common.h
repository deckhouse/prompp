#pragma once

#include <cstdint>
#include <ranges>

namespace series_data {

enum class EncodingType : uint8_t {
  kUnknown,
  kUint32Constant,
  kFloat32Constant,
  kDoubleConstant,
  kTwoDoubleConstant,
  kAscIntegerValuesGorilla,
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
  return (encoding_type == EncodingType::kUint32Constant) || (encoding_type == EncodingType::kFloat32Constant) ||
         (encoding_type == EncodingType::kDoubleConstant) || (encoding_type == EncodingType::kTwoDoubleConstant);
}

constexpr PROMPP_ALWAYS_INLINE bool is_gorilla_based_encoder(EncodingType encoding_type) noexcept {
  return (encoding_type == EncodingType::kAscIntegerValuesGorilla) || (encoding_type == EncodingType::kValuesGorilla) ||
         (encoding_type == EncodingType::kGorilla);
}

constexpr PROMPP_ALWAYS_INLINE bool is_variant_encoder(EncodingType encoding_type) noexcept {
  return (encoding_type == EncodingType::kDoubleConstant) || (encoding_type == EncodingType::kTwoDoubleConstant) ||
         (encoding_type == EncodingType::kAscIntegerValuesGorilla) || (encoding_type == EncodingType::kValuesGorilla);
}

}  // namespace series_data