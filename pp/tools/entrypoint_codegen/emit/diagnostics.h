#pragma once

#include "diagnostics/diagnostics.h"
#include "facts/entrypoint_facts.h"

#include <iosfwd>

namespace entrypoint_codegen::emit {

void write_diagnostics(std::ostream& out, const facts::EntrypointFacts& facts, const diagnostics::DiagnosticSet& diagnostic_set);

}  // namespace entrypoint_codegen::emit
