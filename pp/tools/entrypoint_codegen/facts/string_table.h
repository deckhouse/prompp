#pragma once

#include "facts.h"

#include <cstdint>
#include <memory_resource>
#include <string_view>

namespace epgen::facts {

class StringTable {
 public:
  explicit StringTable(std::pmr::memory_resource* memory_resource = std::pmr::get_default_resource());
  ~StringTable();

  StringTable(StringTable&&) noexcept;
  StringTable& operator=(StringTable&&) noexcept;

  StringTable(const StringTable&) = delete;
  StringTable& operator=(const StringTable&) = delete;

  StringId add(std::string_view value);
  [[nodiscard]] std::string_view get(StringId id) const;

  [[nodiscard]] uint32_t size() const noexcept;
  [[nodiscard]] bool empty() const noexcept;

 private:
  class Impl;
  Impl* impl_{};
  std::pmr::memory_resource* memory_resource_{};
};

}  // namespace epgen::facts