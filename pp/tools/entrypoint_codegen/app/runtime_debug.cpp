#include "app/runtime_debug.h"

#include <optional>
#include <string>

namespace entrypoint_codegen::app {

void append_runtime_debug_diagnostics(facts::EntrypointFacts& facts, RuntimeDebugSnapshot snapshot) {
  const facts::SourceFileId source_file = facts.add_source_file("<runtime>");
  const std::string message = "App PMR allocations: allocated=" + std::to_string(snapshot.allocated_bytes) +
                              " deallocated=" + std::to_string(snapshot.deallocated_bytes) +
                              " peak_live=" + std::to_string(snapshot.peak_live_bytes) + " bytes";
  facts.add_diagnostic(facts::Diagnostic{
      .code = facts::DiagnosticCode::kRuntimeMemoryUsage,
      .message = facts.add_string(message),
      .severity = facts::Severity::kInfo,
      .function = std::nullopt,
      .location = facts::SourceLocation{
          .file = source_file,
          .line = 0,
          .column = 0,
      },
  });
}

}  // namespace entrypoint_codegen::app
