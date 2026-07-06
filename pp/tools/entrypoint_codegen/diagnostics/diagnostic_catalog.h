#pragma once

#include "diagnostics/diagnostics.h"

#include <string_view>

namespace entrypoint_codegen::diagnostics {

std::string_view diagnostic_code_name(DiagnosticCode code);
std::string_view diagnostic_default_message(DiagnosticCode code);
std::string_view diagnostic_message(const Diagnostic& diagnostic);
std::string_view severity_name(Severity severity);

}  // namespace entrypoint_codegen::diagnostics
