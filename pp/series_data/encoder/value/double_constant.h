#pragma once

#include <cstdint>

#include "bare_bones/preprocess.h"
#include "bare_bones/type_traits.h"
#include "series_data/common.h"
#include "series_data/encoder/numeric.h"

namespace series_data::encoder::value {

class DoubleConstantEncoder {
 public:
  explicit DoubleConstantEncoder(double value) : value_(std::bit_cast<uint64_t>(value)) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_actual(const EncodingState& state, double value) const noexcept {
    return is_values_strictly_equal(value, last_value(state));
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool encode(EncodingState& state, double value) const noexcept {
    if (!state.has_last_stalenan && !BareBones::Encoding::Gorilla::isstalenan(value)) [[likely]] {
      return is_actual(state, value);
    }
    if (BareBones::Encoding::Gorilla::isstalenan(value)) {
      state.has_last_stalenan = true;
    }
    return BareBones::Encoding::Gorilla::isstalenan(value);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double value() const noexcept { return std::bit_cast<double>(value_); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE double last_value(const EncodingState& state) const noexcept {
    if (state.has_last_stalenan) [[unlikely]] {
      return BareBones::Encoding::Gorilla::STALE_NAN;
    }
    return value();
  }

 private:
  const uint64_t value_;
};

}  // namespace series_data::encoder::value

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::value::DoubleConstantEncoder> : std::true_type {};
