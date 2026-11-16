#pragma once

#include <compare>
#include <concepts>
#include <cstdint>
#include <limits>

namespace epgen::facts {

template <class Tag, std::unsigned_integral UInt = uint32_t>
class TaggedIndex {
 public:
  using tag_type = Tag;
  using value_type = UInt;

  static constexpr UInt kInvalidValue = std::numeric_limits<UInt>::max();

  constexpr TaggedIndex() noexcept = default;
  constexpr explicit TaggedIndex(UInt value) noexcept : value_(value) {}

  [[nodiscard]] static constexpr TaggedIndex invalid() noexcept { return TaggedIndex(); }

  [[nodiscard]] constexpr bool is_valid() const noexcept { return value_ != kInvalidValue; }
  [[nodiscard]] constexpr UInt get() const noexcept { return value_; }
  constexpr explicit operator UInt() const noexcept { return value_; }
  constexpr explicit operator bool() const noexcept { return is_valid(); }

  constexpr auto operator<=>(const TaggedIndex&) const noexcept = default;

 private:
  UInt value_ = kInvalidValue;
};

}  // namespace epgen::facts
