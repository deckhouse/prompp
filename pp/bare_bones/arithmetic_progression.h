#pragma once

#include <cassert>
#include <compare>
#include <cstdint>
#include <iterator>
#include <limits>
#include <ranges>

namespace BareBones {

// All values must fit uint32_t. Iterators retain their state after the view is destroyed.
class ArithmeticProgression : public std::ranges::view_interface<ArithmeticProgression> {
 public:
  class Iterator {
   public:
    using value_type = uint32_t;
    // End distances must represent the full uint32_t range.
    using difference_type = int64_t;
    using iterator_category = std::random_access_iterator_tag;
    using iterator_concept = std::random_access_iterator_tag;

    Iterator() = default;
    constexpr Iterator(uint32_t first, int64_t step, int64_t position) noexcept
        : value_(advance(first, static_cast<uint32_t>(step), static_cast<uint32_t>(position))), step_(step), position_(position) {}
    [[nodiscard]] constexpr value_type operator*() const noexcept { return value_; }
    [[nodiscard]] constexpr value_type operator[](difference_type n) const noexcept { return *(*this + n); }
    constexpr Iterator& operator++() noexcept {
      value_ = advance(value_, static_cast<uint32_t>(step_));
      ++position_;
      return *this;
    }
    constexpr Iterator operator++(int) noexcept {
      auto copy = *this;
      ++*this;
      return copy;
    }
    constexpr Iterator& operator--() noexcept {
      value_ = advance(value_, static_cast<uint32_t>(step_), uint32_t{0} - 1);
      --position_;
      return *this;
    }
    constexpr Iterator operator--(int) noexcept {
      auto copy = *this;
      --*this;
      return copy;
    }
    constexpr Iterator& operator+=(difference_type n) noexcept {
      value_ = advance(value_, static_cast<uint32_t>(step_), static_cast<uint32_t>(n));
      position_ += n;
      return *this;
    }
    constexpr Iterator& operator-=(difference_type n) noexcept {
      value_ = advance(value_, static_cast<uint32_t>(step_), uint32_t{0} - static_cast<uint32_t>(n));
      position_ -= n;
      return *this;
    }
    friend constexpr Iterator operator+(Iterator it, difference_type n) noexcept { return it += n; }
    friend constexpr Iterator operator+(difference_type n, Iterator it) noexcept { return it += n; }
    friend constexpr Iterator operator-(Iterator it, difference_type n) noexcept { return it -= n; }
    // Comparisons and distances require iterators from the same progression.
    friend constexpr difference_type operator-(const Iterator& a, const Iterator& b) noexcept { return a.position_ - b.position_; }
    friend constexpr bool operator==(const Iterator& a, const Iterator& b) noexcept { return a.position_ == b.position_; }
    friend constexpr auto operator<=>(const Iterator& a, const Iterator& b) noexcept { return a.position_ <=> b.position_; }

   private:
    // Unsigned arithmetic preserves negative steps modulo 2^32.
    static constexpr uint32_t advance(uint32_t value, uint32_t step, uint32_t positions = 1) noexcept { return value + step * positions; }

    uint32_t value_{};
    int64_t step_{};
    int64_t position_{};
  };

  ArithmeticProgression() = default;
  constexpr ArithmeticProgression(uint32_t first, int64_t step, uint32_t size) noexcept
      : first_(size == 0 ? 0 : first), size_(size), step_(size > 1 ? step : 0) {
    assert(size <= 1 ||
           (step_ >= 0 ? step_ <= (std::numeric_limits<uint32_t>::max() - first_) / (size - 1) : step_ >= -static_cast<int64_t>(first_ / (size - 1))));
  }
  [[nodiscard]] constexpr uint32_t first() const noexcept {
    assert(size_ != 0);
    return first_;
  }
  [[nodiscard]] constexpr int64_t step() const noexcept { return step_; }
  [[nodiscard]] constexpr uint32_t size() const noexcept { return size_; }
  [[nodiscard]] constexpr bool empty() const noexcept { return size_ == 0; }
  [[nodiscard]] constexpr Iterator begin() const noexcept { return {first_, step_, 0}; }
  [[nodiscard]] constexpr Iterator end() const noexcept { return {first_, step_, size_}; }
  friend constexpr bool operator==(const ArithmeticProgression& a, const ArithmeticProgression& b) noexcept {
    return a.first_ == b.first_ && a.size_ == b.size_ && a.step_ == b.step_;
  }

 private:
  uint32_t first_{};
  uint32_t size_{};
  int64_t step_{};
};

static_assert(std::ranges::random_access_range<ArithmeticProgression>);
static_assert(std::ranges::sized_range<ArithmeticProgression>);
static_assert(std::ranges::view<ArithmeticProgression>);
}  // namespace BareBones

namespace std::ranges {
template <>
inline constexpr bool enable_borrowed_range<BareBones::ArithmeticProgression> = true;
}
