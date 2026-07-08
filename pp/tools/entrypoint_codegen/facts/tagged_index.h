#pragma once

#include <compare>
#include <concepts>
#include <cstdint>

namespace epgen::facts {

template <class Tag, std::unsigned_integral UInt = uint32_t>
class TaggedIndex {
 public:
  using tag_type = Tag;
  using value_type = UInt;

  constexpr TaggedIndex() noexcept = default;
  constexpr explicit TaggedIndex(UInt value) noexcept : value_(value) {}

  [[nodiscard]] constexpr UInt get() const noexcept { return value_; }
  constexpr explicit operator UInt() const noexcept { return value_; }

  constexpr auto operator<=>(const TaggedIndex&) const noexcept = default;

 private:
  UInt value_{};
};

}  // namespace epgen::facts