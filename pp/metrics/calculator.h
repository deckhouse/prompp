#pragma once

#include "bare_bones/memory.h"
#include "counter.h"

namespace metrics {

class MemoryAllocationsManualCalculator {
 public:
  PROMPP_ALWAYS_INLINE explicit MemoryAllocationsManualCalculator(Counter& counter) : counter_(counter) {}

  PROMPP_ALWAYS_INLINE ~MemoryAllocationsManualCalculator() {
    if (is_started_) [[likely]] {
      counter_.inc(BareBones::allocations_count);
    }
  }

  PROMPP_ALWAYS_INLINE void start() noexcept {
    if (!is_started_) [[likely]] {
      BareBones::allocations_count = 0;
      is_started_ = true;
    }
  }

 private:
  Counter& counter_;
  bool is_started_{};
};

class MemoryAllocationsCalculator {
 public:
  PROMPP_ALWAYS_INLINE explicit MemoryAllocationsCalculator(Counter& counter) : counter_(counter) { BareBones::allocations_count = 0; }

  PROMPP_ALWAYS_INLINE ~MemoryAllocationsCalculator() { counter_.inc(BareBones::allocations_count); }

 private:
  Counter& counter_;
};

}  // namespace metrics