#include "app/run.h"

#include "app/analysis.h"
#include "app/memory_tracking.h"
#include "app/output.h"
#include "diagnostics/diagnostics.h"

namespace epgen::app {

RunReport run(const RunOptions& options) {
  TrackingMemoryResource memory_resource;
  AnalysisResult analysis = analyze_entrypoints(options.analysis, &memory_resource);
  if (options.runtime.debug_diagnostics) {
    append_runtime_diagnostics(analysis, memory_resource.snapshot());
  }

  write_optional_output(options.output, analysis.facts, analysis.diagnostics);

  const diagnostics::SeverityCounts diagnostic_counts = diagnostics::count_by_severity(analysis.diagnostics);
  const MemoryUsageSnapshot memory_usage = memory_resource.snapshot();
  return RunReport{
      .decision = diagnostic_counts.has_errors() ? ExitDecision::kAnalysisFailed : ExitDecision::kSuccess,
      .diagnostics = diagnostic_counts,
      .memory_usage = memory_usage,
  };
}

}  // namespace epgen::app
