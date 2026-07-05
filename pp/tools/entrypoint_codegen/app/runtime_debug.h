#pragma once

#include "facts/entrypoint_facts.h"

#include <cstddef>

namespace entrypoint_codegen::app {

struct RuntimeDebugSnapshot {
  size_t allocated_bytes;
  size_t deallocated_bytes;
  size_t peak_live_bytes;
};

void append_runtime_debug_diagnostics(facts::EntrypointFacts& facts, RuntimeDebugSnapshot snapshot);

}  // namespace entrypoint_codegen::app
