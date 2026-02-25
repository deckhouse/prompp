#pragma once

#include <memory>

#include "preprocess.h"

#if JEMALLOC_AVAILABLE
#include <jemalloc/jemalloc.h>
#endif

namespace BareBones {

template <class Reallocator>
concept ReallocatorInterface = requires(Reallocator reallocator, void* memory) {
  { Reallocator::allocation_size(size_t()) } -> std::same_as<size_t>;
  { Reallocator::reallocate(memory, size_t()) } -> std::same_as<void*>;
  { Reallocator::free(memory) } -> std::same_as<void>;
};

using ArenaIndex = uint32_t;

static constexpr ArenaIndex kInvalidArenaIndex = std::numeric_limits<ArenaIndex>::max();

template <class ArenaReallocator>
concept ArenaAllocatorInterface = requires(ArenaReallocator reallocator) {
  { ArenaReallocator::create_arena() } -> std::same_as<ArenaIndex>;
  { ArenaReallocator::destroy_arena(ArenaIndex()) };
  { ArenaReallocator::thread_arena_guard(ArenaIndex()) };
  { ArenaReallocator::arena_allocated_memory(ArenaIndex()) } -> std::convertible_to<size_t>;
};

struct DefaultReallocator {
  PROMPP_ALWAYS_INLINE static size_t allocation_size(size_t needed_size) noexcept {
#if JEMALLOC_AVAILABLE
    return nallocx(needed_size, 0);
#else
    return needed_size;
#endif
  }

  PROMPP_ALWAYS_INLINE static void* allocate(size_t size) { return std::malloc(size); }

  PROMPP_ALWAYS_INLINE static void* reallocate(void* memory, size_t size) { return std::realloc(memory, size); }

  PROMPP_ALWAYS_INLINE static void free(void* memory) { return std::free(memory); }
};

static_assert(ReallocatorInterface<DefaultReallocator>);

template <class T, ReallocatorInterface Reallocator = DefaultReallocator, class CounterType = size_t>
class Allocator {
 public:
  using value_type = T;

  explicit constexpr Allocator(CounterType& allocated_memory) : allocated_memory_(allocated_memory) {}
  constexpr Allocator(const Allocator&) = default;
  template <class AnyType, ReallocatorInterface AnyReallocator, class CounterType2>
  explicit constexpr Allocator(const Allocator<AnyType, AnyReallocator, CounterType2>& other) : allocated_memory_(other.allocated_memory_) {}
  constexpr Allocator(Allocator&&) noexcept = default;

  constexpr Allocator& operator=(const Allocator&) = delete;
  constexpr Allocator& operator=(Allocator&&) noexcept = delete;
  constexpr bool operator==(const Allocator& other) const noexcept { return &allocated_memory_ == &other.allocated_memory_; };

  [[nodiscard]] PROMPP_ALWAYS_INLINE constexpr T* allocate(std::size_t n) {
    allocated_memory_ += static_cast<CounterType>(n * sizeof(T));

    auto r = static_cast<T*>(Reallocator::allocate(n * sizeof(T)));
    std::uninitialized_default_construct_n(r, n);
    return r;
  }
  PROMPP_ALWAYS_INLINE void deallocate(T* p, std::size_t n) {
    std::destroy_n(p, n);
    Reallocator::free(p);

    allocated_memory_ -= static_cast<CounterType>(n * sizeof(T));
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE CounterType allocated_memory() const noexcept { return allocated_memory_; }

 private:
  template <class AnyType, ReallocatorInterface AnyReallocator, class CounterType2>
  friend class Allocator;

  CounterType& allocated_memory_;
};

}  // namespace BareBones
