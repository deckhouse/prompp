#pragma once

#include "app/memory_usage.h"

#include <cstddef>
#include <memory_resource>

namespace epgen::app {

class TrackingMemoryResource : public std::pmr::memory_resource {
 public:
  explicit TrackingMemoryResource(std::pmr::memory_resource* upstream = std::pmr::get_default_resource());

  [[nodiscard]] MemoryUsageSnapshot snapshot() const noexcept;

 private:
  void* do_allocate(size_t bytes, size_t alignment) override;
  void do_deallocate(void* pointer, size_t bytes, size_t alignment) override;
  bool do_is_equal(const std::pmr::memory_resource& other) const noexcept override;

  std::pmr::memory_resource* upstream_;
  size_t allocated_bytes_ = 0;
  size_t deallocated_bytes_ = 0;
  size_t live_bytes_ = 0;
  size_t peak_live_bytes_ = 0;
};

}  // namespace epgen::app
