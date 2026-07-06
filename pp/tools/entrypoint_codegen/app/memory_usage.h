#pragma once

#include <cstddef>

namespace entrypoint_codegen::app {

struct MemoryUsageSnapshot {
  size_t allocated_bytes = 0;
  size_t deallocated_bytes = 0;
  size_t peak_live_bytes = 0;
};

}  // namespace entrypoint_codegen::app
