#pragma once

#include "app/options.h"
#include "diagnostics/diagnostics.h"

namespace epgen::app {

struct RunReport {
  ExitDecision decision;
  diagnostics::SeverityCounts diagnostics;
};

RunReport run(const RunOptions& options);

}  // namespace epgen::app
