#pragma once

#include <cstdint>
#include <limits>

namespace series_index {

enum class SymbolSource : uint8_t {
  kCurrent = 0,
  kSnapshot = 1,
};

inline constexpr uint32_t kKeyOnlyValueId = std::numeric_limits<uint32_t>::max();

}  // namespace series_index
