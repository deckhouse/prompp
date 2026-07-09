#pragma once

#include "app/memory_usage.h"
#include "diagnostics/diagnostics.h"

namespace epgen::facts {
class FactArena;
}

namespace epgen::app {

void append_runtime_debug_diagnostics(diagnostics::DiagnosticSet& diagnostic_set, facts::FactArena& facts, MemoryUsageSnapshot snapshot);

}  // namespace epgen::app
