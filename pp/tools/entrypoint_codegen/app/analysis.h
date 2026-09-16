#pragma once

#include "app/options.h"
#include "diagnostics/diagnostics.h"
#include "facts/fact_store.h"

namespace epgen::app {

struct AnalysisResult {
  facts::FactStore facts;
  diagnostics::DiagnosticSet diagnostics;
};

AnalysisResult analyze_entrypoints(const AnalysisOptions& options);

}  // namespace epgen::app
