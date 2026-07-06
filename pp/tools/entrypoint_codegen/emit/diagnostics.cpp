#include "emit/diagnostics.h"

#include "diagnostics/diagnostic_catalog.h"

#include <ostream>

namespace entrypoint_codegen::emit {

void write_diagnostics(std::ostream& out, const facts::EntrypointFacts& facts, const diagnostics::DiagnosticSet& diagnostic_set) {
  for (const diagnostics::Diagnostic& diagnostic : diagnostic_set.diagnostics()) {
    if (diagnostic.location.has_value()) {
      const facts::SourceLocation location = *diagnostic.location;
      out << facts.string(facts.source_file(location.file).path) << ":" << location.line << ":" << location.column << ": ";
    }
    out << diagnostics::severity_name(diagnostic.severity) << ": " << diagnostics::diagnostic_message(diagnostic) << " ["
        << diagnostics::diagnostic_code_name(diagnostic.code) << "]\n";
  }
}

}  // namespace entrypoint_codegen::emit
