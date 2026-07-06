#pragma once

#include "app/memory_usage.h"
#include "app/options.h"
#include "diagnostics/diagnostics.h"

namespace entrypoint_codegen::app {

struct RunReport {
  ExitDecision decision;
  diagnostics::SeverityCounts diagnostics;
  MemoryUsageSnapshot memory_usage;
};

RunReport run(const RunOptions& options);

}  // namespace entrypoint_codegen::app
