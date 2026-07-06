#include "app/memory_tracking.h"

namespace entrypoint_codegen::app {

TrackingMemoryResource::TrackingMemoryResource(std::pmr::memory_resource* upstream) : upstream_(upstream) {}

MemoryUsageSnapshot TrackingMemoryResource::snapshot() const noexcept {
  return MemoryUsageSnapshot{
      .allocated_bytes = allocated_bytes_,
      .deallocated_bytes = deallocated_bytes_,
      .peak_live_bytes = peak_live_bytes_,
  };
}

void* TrackingMemoryResource::do_allocate(size_t bytes, size_t alignment) {
  void* result = upstream_->allocate(bytes, alignment);
  allocated_bytes_ += bytes;
  live_bytes_ += bytes;
  if (live_bytes_ > peak_live_bytes_) {
    peak_live_bytes_ = live_bytes_;
  }
  return result;
}

void TrackingMemoryResource::do_deallocate(void* pointer, size_t bytes, size_t alignment) {
  upstream_->deallocate(pointer, bytes, alignment);
  deallocated_bytes_ += bytes;
  live_bytes_ -= bytes;
}

bool TrackingMemoryResource::do_is_equal(const std::pmr::memory_resource& other) const noexcept {
  return this == &other;
}

}  // namespace entrypoint_codegen::app
