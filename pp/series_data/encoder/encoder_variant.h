#pragma once

#include <cassert>

#include "value/asc_integer.h"
#include "value/double_constant.h"
#include "value/two_double_constant.h"
#include "value/values_gorilla.h"

namespace series_data::encoder {

union PROMPP_ATTRIBUTE_PACKED EncoderVariant {
  value::DoubleConstantEncoder double_constant{0};
  value::TwoDoubleConstantEncoder two_double_constant;
  value::AscIntegerEncoder asc_integer;
  value::ValuesGorillaEncoder values_gorilla;

  void destroy(EncodingType encoding_type) {
    switch (encoding_type) {
      case EncodingType::kDoubleConstant:
        std::destroy_at(&double_constant);
        break;
      case EncodingType::kTwoDoubleConstant:
        std::destroy_at(&two_double_constant);
        break;
      case EncodingType::kAscInteger:
        std::destroy_at(&asc_integer);
        break;
      case EncodingType::kValuesGorilla:
        std::destroy_at(&values_gorilla);
        break;
      default:
        assert(encoding_type != EncodingType::kDoubleConstant && "Unsupported encoding type in EncoderVariant");
    }
  }

  template <EncodingType E, class... Args>
  void construct(Args&&... args) {
    using enum EncodingType;
    if constexpr (E == kDoubleConstant) {
      std::construct_at(&double_constant, std::forward<Args>(args)...);
    } else if constexpr (E == kTwoDoubleConstant) {
      std::construct_at(&two_double_constant, std::forward<Args>(args)...);
    } else if constexpr (E == kAscInteger) {
      std::construct_at(&asc_integer, std::forward<Args>(args)...);
    } else if constexpr (E == kValuesGorilla) {
      std::construct_at(&values_gorilla, std::forward<Args>(args)...);
    } else {
      static_assert(false, "Unsupported encoding type in EncoderVariant");
    }
  }

  uint32_t allocated_memory(EncodingType encoding_type) const noexcept {
    switch (encoding_type) {
      case EncodingType::kDoubleConstant:
      case EncodingType::kTwoDoubleConstant:
        return 0;
      case EncodingType::kAscInteger:
        return asc_integer.allocated_memory();
      case EncodingType::kValuesGorilla:
        return values_gorilla.allocated_memory();
      default:
        assert(encoding_type != EncodingType::kDoubleConstant && "Unsupported encoding type in EncoderVariant");
        return 0;
    }
  }

  EncoderVariant() {}
  ~EncoderVariant() {}
};

}  // namespace series_data::encoder