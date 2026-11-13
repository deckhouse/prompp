#pragma once

#include "bare_bones/vector.h"
#include "numeric.h"
#include "primitives/primitives.h"

namespace series_data::encoder {

struct Sample {
  PromPP::Primitives::Timestamp timestamp{};
  double value{};

  PROMPP(inline) bool operator==(const Sample& other) const noexcept {
    return timestamp == other.timestamp && value::is_values_strictly_equal(value, other.value);
  }
  PROMPP(inline) bool operator<(const Sample& other) const noexcept { return timestamp < other.timestamp; }
  PROMPP(inline) bool operator<=(const Sample& other) const noexcept { return *this < other || *this == other; }
  PROMPP(inline) bool operator>(const Sample& other) const noexcept { return timestamp > other.timestamp; }
  PROMPP(inline) bool operator>=(const Sample& other) const noexcept { return *this > other || *this == other; }
};

using SampleList = BareBones::Vector<Sample>;

}  // namespace series_data::encoder