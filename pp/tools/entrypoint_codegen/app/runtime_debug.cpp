#include "app/runtime_debug.h"

#include <optional>
#include <string>

namespace epgen::app {

void append_runtime_debug_diagnostics(diagnostics::DiagnosticSet& diagnostic_set, MemoryUsageSnapshot snapshot) {
  const std::string message = "App PMR allocations: allocated=" + std::to_string(snapshot.allocated_bytes) +
                              " deallocated=" + std::to_string(snapshot.deallocated_bytes) + " peak_live=" + std::to_string(snapshot.peak_live_bytes) +
                              " bytes";
  diagnostic_set.add(diagnostics::Diagnostic{
      .code = diagnostics::DiagnosticCode::kRuntimeMemoryUsage,
      .message = message,
      .severity = diagnostics::Severity::kInfo,
      .function = std::nullopt,
      .location = std::nullopt,
  });
}

}  // namespace epgen::app
