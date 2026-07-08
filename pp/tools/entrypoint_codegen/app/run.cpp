#include "app/run.h"

#include "app/memory_tracking.h"
#include "app/runtime_debug.h"
#include "clang_adapter/parse.h"
#include "contract/entrypoint_contract.h"
#include "diagnostics/diagnostics.h"
#include "emit/report.h"

#include <filesystem>
#include <fstream>
#include <iostream>
#include <stdexcept>

namespace epgen::app {

namespace {

void write_json_output(const OutputOptions& options, const facts::FactArena& facts, const diagnostics::DiagnosticSet& diagnostic_set) {
  if (options.output_path.has_parent_path()) {
    std::filesystem::create_directories(options.output_path.parent_path());
  }

  std::ofstream output(options.output_path, std::ios::trunc);
  if (!output) {
    throw std::runtime_error("failed to open output file: " + options.output_path.string());
  }
  emit::write_report(output, emit::ReportFormat::kJson, facts, diagnostic_set);
}

void write_lint_output(const OutputOptions& options, const facts::FactArena& facts, const diagnostics::DiagnosticSet& diagnostic_set) {
  std::ostream& output = options.diagnostics_output == nullptr ? std::cout : *options.diagnostics_output;
  emit::write_report(output, emit::ReportFormat::kCompilerDiagnostics, facts, diagnostic_set);
}

}  // namespace

RunReport run(const RunOptions& options) {
  TrackingMemoryResource memory_resource;
  diagnostics::DiagnosticSet diagnostic_set(&memory_resource);
  facts::FactArena facts = clang_adapter::parse_files(
      clang_adapter::ParseOptions{
          .source_files = options.analysis.source_files,
          .clang_args = options.analysis.clang_args,
          .memory_resource = &memory_resource,
      },
      diagnostic_set);

  contract::validate_entrypoints(facts, diagnostic_set);

  if (options.runtime.debug_diagnostics) {
    append_runtime_debug_diagnostics(diagnostic_set, memory_resource.snapshot());
  }

  switch (options.output.output_mode) {
    case OutputMode::kJson: {
      write_json_output(options.output, facts, diagnostic_set);
      break;
    }
    case OutputMode::kLint: {
      write_lint_output(options.output, facts, diagnostic_set);
      break;
    }
    case OutputMode::kCheck: {
      break;
    }
  }

  const diagnostics::SeverityCounts diagnostic_counts = diagnostics::count_by_severity(diagnostic_set);
  const MemoryUsageSnapshot memory_usage = memory_resource.snapshot();
  return RunReport{
      .decision = diagnostic_counts.has_errors() ? ExitDecision::kAnalysisFailed : ExitDecision::kSuccess,
      .diagnostics = diagnostic_counts,
      .memory_usage = memory_usage,
  };
}

}  // namespace epgen::app
