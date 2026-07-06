#pragma once

#include "diagnostics/diagnostics.h"

#include <cstddef>

namespace entrypoint_codegen::app {

struct RuntimeDebugSnapshot {
  size_t allocated_bytes;
  size_t deallocated_bytes;
  size_t peak_live_bytes;
};

void append_runtime_debug_diagnostics(diagnostics::DiagnosticSet& diagnostic_set, RuntimeDebugSnapshot snapshot);

}  // namespace entrypoint_codegen::app
