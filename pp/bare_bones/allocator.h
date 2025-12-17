#pragma once

#include <memory>

#include "memory.h"
#include "preprocess.h"

namespace BareBones {

template <class T, class CounterType = size_t>
class Allocator {
 public:
  using value_type = T;

  explicit constexpr Allocator(CounterType& allocated_memory) : allocated_memory_(allocated_memory) {}
  constexpr Allocator(const Allocator&) = default;
  template <class AnyType, class CounterType2>
  explicit constexpr Allocator(const Allocator<AnyType, CounterType2>& other) : allocated_memory_(other.allocated_memory_) {}
  constexpr Allocator(Allocator&&) noexcept = default;

  constexpr Allocator& operator=(const Allocator&) = delete;
  constexpr Allocator& operator=(Allocator&&) noexcept = delete;
  constexpr bool operator==(const Allocator& other) const noexcept { return &allocated_memory_ == &other.allocated_memory_; };

  [[nodiscard]] PROMPP_ALWAYS_INLINE constexpr T* allocate(std::size_t n) {
    auto r = static_cast<T*>(DefaultReallocator::allocate(n * sizeof(T)));
    std::uninitialized_default_construct_n(r, n);
    return r;
  }
  PROMPP_ALWAYS_INLINE void deallocate(T* p, std::size_t n) {
    std::destroy_n(p, n);
    DefaultReallocator::free(p);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE CounterType allocated_memory() const noexcept { return allocated_memory_; }

 private:
  template <class AnyType, class CounterType2>
  friend class Allocator;

  CounterType& allocated_memory_;
};

}  // namespace BareBones
