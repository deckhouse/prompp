#include "app/analysis.h"

#include "clang_adapter/parse.h"
#include "contract/entrypoint_contract.h"

#include <utility>

namespace epgen::app {

namespace {}  // namespace

AnalysisResult analyze_entrypoints(const AnalysisOptions& options) {
  diagnostics::DiagnosticSet diagnostics;
  facts::FactStore facts =
      clang_adapter::parse_files(clang_adapter::ParseOptions{.source_files = options.source_files, .clang_args = options.clang_args}, diagnostics);

  contract::validate_contract(facts, diagnostics);

  return AnalysisResult{
      .facts = std::move(facts),
      .diagnostics = std::move(diagnostics),
  };
}

}  // namespace epgen::app
