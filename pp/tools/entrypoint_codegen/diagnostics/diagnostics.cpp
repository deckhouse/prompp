#include "diagnostics/diagnostics.h"

#include <utility>

namespace entrypoint_codegen::diagnostics {

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

}  // namespace entrypoint_codegen::diagnostics
