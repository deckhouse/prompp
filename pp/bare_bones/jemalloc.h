#pragma once

#include <mutex>
#include <utility>
#include <vector>

#include "allocator.h"

#if PROMPP(feature, jemalloc)
#  include <jemalloc/jemalloc.h>

#  include <unistd.h>
#endif

namespace BareBones::jemalloc {

PROMPP(inline) void refresh_stats() noexcept {
#if PROMPP(feature, jemalloc)
  uint64_t epoch = 1;
  size_t sz = sizeof(epoch);
  mallctl("epoch", &epoch, &sz, &epoch, sz);
#endif
}

#if PROMPP(feature, jemalloc)

inline const auto kPageSize = sysconf(_SC_PAGESIZE);

template <class Object>
struct ArenaReallocator;

struct FreeArenas {
  static double releases_total;
  static double released_bytes_total;
  static double released_bytes_max;

 private:
  static inline std::vector<ArenaIndex> free_arenas;
  static inline std::mutex free_arenas_mutex;

  template <class Object>
  friend struct ArenaReallocator;

  static ArenaIndex get() noexcept {
    std::scoped_lock lock(free_arenas_mutex);
    if (!free_arenas.empty()) {
      const auto result = free_arenas.back();
      free_arenas.pop_back();
      return result;
    }

    return kInvalidArenaIndex;
  }

  static void add(ArenaIndex arena_index, size_t arena_memory_bytes) noexcept {
    std::scoped_lock lock(free_arenas_mutex);
    free_arenas.push_back(arena_index);

    ++releases_total;
    released_bytes_total += static_cast<double>(arena_memory_bytes);
    released_bytes_max = std::max(released_bytes_max, static_cast<double>(arena_memory_bytes));
  }
};

inline double FreeArenas::releases_total{};
inline double FreeArenas::released_bytes_total{};
inline double FreeArenas::released_bytes_max{};

template <class Object>
struct ArenaReallocator {
  thread_local static ArenaIndex thread_arena_index;

  static PROMPP(inline) size_t allocation_size(size_t needed_size) noexcept {
    return nallocx(needed_size, MALLOCX_ARENA(thread_arena_index) | MALLOCX_TCACHE_NONE);
  }

  static PROMPP(inline) void* allocate(size_t size) { return mallocx(size, MALLOCX_ARENA(thread_arena_index) | MALLOCX_TCACHE_NONE); }

  static PROMPP(inline) void* reallocate(void* memory, size_t size) {
    if (memory == nullptr) [[unlikely]] {
      return mallocx(size, MALLOCX_ARENA(thread_arena_index) | MALLOCX_TCACHE_NONE);
    }

    return rallocx(memory, size, MALLOCX_ARENA(thread_arena_index) | MALLOCX_TCACHE_NONE);
  }

  static PROMPP(inline) void free(void* memory) {
    if (memory != nullptr) [[likely]] {
      return dallocx(memory, MALLOCX_ARENA(thread_arena_index) | MALLOCX_TCACHE_NONE);
    }
  }

  static PROMPP(inline) size_t arena_allocated_memory(ArenaIndex arena_index) noexcept {
    refresh_stats();

    size_t pages{};
    size_t size_len = sizeof(pages);
    mallctl(create_command("stats.arenas.%u.pactive", arena_index).data(), &pages, &size_len, nullptr, 0);

    return pages * kPageSize;
  }

  static PROMPP(inline) ArenaIndex create_arena() noexcept {
    auto arena_index = FreeArenas::get();
    if (arena_index == kInvalidArenaIndex) {
      size_t size = sizeof(arena_index);
      mallctl("arenas.create", &arena_index, &size, nullptr, 0);
    }

    return arena_index;
  }

  static PROMPP(inline) void release_arena(ArenaIndex arena_index) noexcept {
    const auto bytes_at_release = arena_allocated_memory(arena_index);
    mallctl(create_command("arena.%u.reset", arena_index).data(), nullptr, nullptr, nullptr, 0);
    mallctl(create_command("arena.%u.purge", arena_index).data(), nullptr, nullptr, nullptr, 0);
    FreeArenas::add(arena_index, bytes_at_release);
  }

  static PROMPP(inline) auto thread_arena_guard(ArenaIndex arena_index) noexcept {
    class Guard {
     public:
      explicit Guard(ArenaIndex arena_index) noexcept : previous_arena_index_{std::exchange(thread_arena_index, arena_index)} {}
      ~Guard() { thread_arena_index = previous_arena_index_; }

     private:
      ArenaIndex previous_arena_index_;
    };

    return Guard(arena_index);
  }

 private:
  static PROMPP(inline) auto create_command(const char* command, ArenaIndex arena_index) noexcept {
    std::array<char, 64> buffer;
    snprintf(buffer.data(), buffer.size(), command, arena_index);
    return buffer;
  }
};

template <class Object>
thread_local ArenaIndex ArenaReallocator<Object>::thread_arena_index{kInvalidArenaIndex};

static_assert(ReallocatorInterface<ArenaReallocator<int>>);
static_assert(ArenaAllocatorInterface<ArenaReallocator<int>>);

#endif

};  // namespace BareBones::jemalloc
