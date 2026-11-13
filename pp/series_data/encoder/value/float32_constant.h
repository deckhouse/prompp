#pragma once

#include "bare_bones/preprocess.h"
#include "bare_bones/type_traits.h"
#include "series_data/common.h"
#include "series_data/encoder/numeric.h"

namespace series_data::encoder::value {

class PROMPP(packed) Float32ConstantEncoder {
 public:
  explicit Float32ConstantEncoder(double value) : value_(float_value(value)) {}

  [[nodiscard]] static PROMPP(inline) bool can_be_encoded(double value) noexcept { return is_float(value); }
  [[nodiscard]] PROMPP(inline) bool is_actual(const EncodingState& state, double value) const noexcept {
    return is_values_strictly_equal(value, last_value(state));
  }

  [[nodiscard]] PROMPP(inline) bool encode(EncodingState& state, double value) const noexcept {
    if (!state.has_last_stalenan && !BareBones::Encoding::Gorilla::isstalenan(value)) [[likely]] {
      return is_actual(state, value);
    }
    if (BareBones::Encoding::Gorilla::isstalenan(value)) {
      state.has_last_stalenan = true;
    }
    return BareBones::Encoding::Gorilla::isstalenan(value);
  }

  [[nodiscard]] PROMPP(inline) double value() const noexcept { return value_; }
  [[nodiscard]] PROMPP(inline) double last_value(const EncodingState& state) const noexcept {
    if (state.has_last_stalenan) [[unlikely]] {
      return BareBones::Encoding::Gorilla::STALE_NAN;
    }
    return value();
  }

 private:
  const float value_;

  [[nodiscard]] static PROMPP(inline) float float_value(double value) noexcept { return static_cast<float>(value); }
};

}  // namespace series_data::encoder::value

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::value::Float32ConstantEncoder> : std::true_type {};