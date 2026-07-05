#pragma once

#include "facts/entrypoint_facts.h"
#include "facts/facts.h"

#include <string_view>

namespace entrypoint_codegen::emit {

std::string_view diagnostic_code_name(facts::DiagnosticCode code);
std::string_view diagnostic_default_message(facts::DiagnosticCode code);
std::string_view diagnostic_message(const facts::EntrypointFacts& facts, const facts::Diagnostic& diagnostic);

}  // namespace entrypoint_codegen::emit
