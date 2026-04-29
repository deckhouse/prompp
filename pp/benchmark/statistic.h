#pragma once

#include <algorithm>
#include <ranges>
#include <vector>

#include "bare_bones/preprocess.h"

namespace benchmark {

PROMPP_ALWAYS_INLINE double min_time(const std::vector<double>& v) noexcept {
  if (v.empty()) [[unlikely]] {
    return 0.0;
  }

  return *std::ranges::min_element(v);
}

}  // namespace benchmark
