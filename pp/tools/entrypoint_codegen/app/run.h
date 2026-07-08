#pragma once

#include "app/memory_usage.h"
#include "app/options.h"
#include "diagnostics/diagnostics.h"

namespace epgen::app {

struct RunReport {
  ExitDecision decision;
  diagnostics::SeverityCounts diagnostics;
  MemoryUsageSnapshot memory_usage;
};

RunReport run(const RunOptions& options);

}  // namespace epgen::app
