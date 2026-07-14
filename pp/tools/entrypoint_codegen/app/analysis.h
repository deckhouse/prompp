#pragma once

#include "app/memory_usage.h"
#include "app/options.h"
#include "diagnostics/diagnostics.h"
#include "facts/fact_arena.h"

#include <memory_resource>

namespace epgen::app {

struct AnalysisResult {
  facts::FactArena facts;
  diagnostics::DiagnosticSet diagnostics;
};

AnalysisResult analyze_entrypoints(const AnalysisOptions& options, std::pmr::memory_resource* memory_resource);

void append_runtime_diagnostics(AnalysisResult& result, MemoryUsageSnapshot memory_usage);

}  // namespace epgen::app
