#pragma once

#include <bit>
#include <cmath>
#include <limits>

#include "bare_bones/numeric.h"
#include "bare_bones/preprocess.h"

namespace series_data::encoder::value {

constexpr PROMPP_ALWAYS_INLINE bool is_int(double value) noexcept {
  // INT64_MAX rounds to 2^63 as double, so the upper bound is exclusive.
  return value >= -0x1p63 && value < 0x1p63 && static_cast<double>(static_cast<int64_t>(value)) == value;
}

template <class Number1, class Number2>
constexpr PROMPP_ALWAYS_INLINE bool is_values_strictly_equal(Number1 a, Number2 b) noexcept {
  if constexpr (sizeof(Number1) == sizeof(Number2)) {
    using unsigned_integral = BareBones::numeric::integral_type_for<Number1>;
    return std::bit_cast<unsigned_integral>(a) == std::bit_cast<unsigned_integral>(b);
  }
  return false;
}

constexpr PROMPP_ALWAYS_INLINE bool is_float(double value) noexcept {
  if (std::isfinite(value) && std::abs(value) > std::numeric_limits<float>::max()) {
    return false;
  }
  return is_values_strictly_equal(value, static_cast<double>(static_cast<float>(value)));
}

template <class Value, class Number1, class Number2>
constexpr PROMPP_ALWAYS_INLINE bool is_in_bounds(Value value, Number1 min, Number2 max) noexcept {
  return value >= static_cast<Value>(min) && value <= static_cast<Value>(max);
}

}  // namespace series_data::encoder::value
