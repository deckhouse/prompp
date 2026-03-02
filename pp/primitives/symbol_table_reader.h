#pragma once

#include <cstdint>
#include <string_view>

#include "bare_bones/preprocess.h"

namespace PromPP::Primitives::SnugComposites {

struct SymbolItemType {
  uint32_t pos;
  uint32_t length;
};

struct SymbolTableReader {
  const char* data = nullptr;
  const SymbolItemType* items = nullptr;

  [[nodiscard]] PROMPP_ALWAYS_INLINE std::string_view operator[](uint32_t id) const noexcept {
    const auto& it = items[id];
    return std::string_view(data + it.pos, it.length);
  }
};

}  // namespace PromPP::Primitives::SnugComposites
