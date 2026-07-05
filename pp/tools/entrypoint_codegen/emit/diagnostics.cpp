#include "emit/diagnostics.h"

#include "emit/diagnostic_text.h"

#include <ostream>
#include <string_view>

namespace entrypoint_codegen::emit {

namespace {

std::string_view severity_name(facts::Severity severity) {
  switch (severity) {
    case facts::Severity::kInfo: {
      return "info";
    }
    case facts::Severity::kWarning: {
      return "warning";
    }
    case facts::Severity::kError: {
      return "error";
    }
  }
  return "error";
}

}  // namespace

void write_diagnostics(std::ostream& out, const facts::EntrypointFacts& facts) {
  for (const facts::Diagnostic& diagnostic : facts.diagnostics()) {
    const facts::SourceLocation location = diagnostic.location;
    out << facts.string(facts.source_file(location.file).path) << ":" << location.line << ":" << location.column
        << ": " << severity_name(diagnostic.severity) << ": " << diagnostic_message(facts, diagnostic)
        << " [" << diagnostic_code_name(diagnostic.code) << "]\n";
  }
}

}  // namespace entrypoint_codegen::emit
