#include "app/run.h"

#include "app/analysis.h"
#include "app/output.h"
#include "diagnostics/diagnostics.h"

namespace epgen::app {

RunReport run(const RunOptions& options) {
  AnalysisResult analysis = analyze_entrypoints(options.analysis);

  write_optional_output(options.output, analysis.facts, analysis.diagnostics);

  const diagnostics::SeverityCounts diagnostic_counts = diagnostics::count_by_severity(analysis.diagnostics);
  return RunReport{
      .decision = diagnostic_counts.has_errors() ? ExitDecision::kAnalysisFailed : ExitDecision::kSuccess,
      .diagnostics = diagnostic_counts,
  };
}

}  // namespace epgen::app
