#include "diagnostics/diagnostics.h"

#include <utility>

namespace entrypoint_codegen::diagnostics {

bool SeverityCounts::has_errors() const noexcept {
  return errors != 0;
}

DiagnosticSet::DiagnosticSet(std::pmr::memory_resource* memory_resource) : diagnostics_(memory_resource) {}

void DiagnosticSet::add(Diagnostic diagnostic) {
  diagnostics_.push_back(std::move(diagnostic));
}

std::span<const Diagnostic> DiagnosticSet::diagnostics() const noexcept {
  return diagnostics_;
}

uint32_t DiagnosticSet::count() const noexcept {
  return static_cast<uint32_t>(diagnostics_.size());
}

bool DiagnosticSet::empty() const noexcept {
  return diagnostics_.empty();
}

SeverityCounts count_by_severity(const DiagnosticSet& diagnostic_set) {
  SeverityCounts counts;
  for (const Diagnostic& diagnostic : diagnostic_set.diagnostics()) {
    ++counts.total;
    switch (diagnostic.severity) {
      case Severity::kInfo: {
        ++counts.infos;
        break;
      }
      case Severity::kWarning: {
        ++counts.warnings;
        break;
      }
      case Severity::kError: {
        ++counts.errors;
        break;
      }
    }
  }
  return counts;
}

}  // namespace entrypoint_codegen::diagnostics
