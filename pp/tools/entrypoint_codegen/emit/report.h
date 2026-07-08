#pragma once

#include "diagnostics/diagnostics.h"
#include "facts/fact_arena.h"

#include <cstdint>
#include <iosfwd>

namespace epgen::emit {

enum class ReportFormat : uint8_t {
  kJson,
  kCompilerDiagnostics,
};

void write_report(std::ostream& out, ReportFormat format, const facts::FactArena& facts, const diagnostics::DiagnosticSet& diagnostic_set);

}  // namespace epgen::emit
