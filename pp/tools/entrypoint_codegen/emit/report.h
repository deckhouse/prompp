#pragma once

#include "diagnostics/diagnostics.h"
#include "facts/entrypoint_facts.h"

#include <cstdint>
#include <iosfwd>

namespace entrypoint_codegen::emit {

enum class ReportFormat : uint8_t {
  kJson,
  kCompilerDiagnostics,
};

void write_report(std::ostream& out, ReportFormat format, const facts::EntrypointFacts& facts, const diagnostics::DiagnosticSet& diagnostic_set);

}  // namespace entrypoint_codegen::emit
