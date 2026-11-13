#pragma once

#include <cstdint>

#include "bare_bones/gorilla.h"
#include "bare_bones/preprocess.h"

namespace series_data::encoder::value {

struct PROMPP(packed) ConstantValue {
  double value{BareBones::Encoding::Gorilla::STALE_NAN};
  uint8_t count{};

  [[nodiscard]] PROMPP(inline) bool has_value() const noexcept { return count > 0; }
  [[nodiscard]] PROMPP(inline) bool is_stalenan() const noexcept { return BareBones::Encoding::Gorilla::isstalenan(value); }
};

}  // namespace series_data::encoder::value
