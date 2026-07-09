#include "app/analysis.h"

#include "clang_adapter/parse.h"
#include "contract/entrypoint_contract.h"

#include <string>
#include <utility>

namespace epgen::app {

namespace {

void append_runtime_memory_diagnostic(diagnostics::DiagnosticSet& diagnostic_set, facts::FactArena& facts, MemoryUsageSnapshot snapshot) {
  const std::string message = "App PMR allocations: allocated=" + std::to_string(snapshot.allocated_bytes) +
                              " deallocated=" + std::to_string(snapshot.deallocated_bytes) + " peak_live=" + std::to_string(snapshot.peak_live_bytes) +
                              " bytes";
  diagnostic_set.add(diagnostics::Diagnostic{
      .code = diagnostics::DiagnosticCode::kRuntimeMemoryUsage,
      .message = facts.add_string(message),
      .severity = diagnostics::Severity::kInfo,
  });
}

}  // namespace

AnalysisResult analyze_entrypoints(const AnalysisOptions& options, std::pmr::memory_resource* memory_resource) {
  diagnostics::DiagnosticSet diagnostics(memory_resource);
  facts::FactArena facts = clang_adapter::parse_files(
      clang_adapter::ParseOptions{
          .source_files = options.source_files,
          .clang_args = options.clang_args,
          .memory_resource = memory_resource,
      },
      diagnostics);

  contract::validate_contract(facts, diagnostics);

  return AnalysisResult{
      .facts = std::move(facts),
      .diagnostics = std::move(diagnostics),
  };
}

void append_runtime_diagnostics(AnalysisResult& result, MemoryUsageSnapshot memory_usage) {
  append_runtime_memory_diagnostic(result.diagnostics, result.facts, memory_usage);
}

}  // namespace epgen::app
