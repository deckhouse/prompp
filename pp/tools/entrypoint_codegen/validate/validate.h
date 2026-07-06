#pragma once

#include "diagnostics/diagnostics.h"
#include "facts/entrypoint_facts.h"

namespace entrypoint_codegen::validate {

void validate_entrypoints(const facts::EntrypointFacts& facts, diagnostics::DiagnosticSet& diagnostic_set);

}  // namespace entrypoint_codegen::validate
