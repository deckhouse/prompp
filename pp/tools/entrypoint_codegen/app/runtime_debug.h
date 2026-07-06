#pragma once

#include "app/memory_usage.h"
#include "diagnostics/diagnostics.h"

namespace entrypoint_codegen::app {

void append_runtime_debug_diagnostics(diagnostics::DiagnosticSet& diagnostic_set, MemoryUsageSnapshot snapshot);

}  // namespace entrypoint_codegen::app
