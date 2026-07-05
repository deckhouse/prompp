#include "emit/diagnostics.h"

#include <gtest/gtest.h>

#include <optional>
#include <sstream>

namespace {

using entrypoint_codegen::facts::Diagnostic;
using entrypoint_codegen::facts::DiagnosticCode;
using entrypoint_codegen::facts::EntrypointFacts;
using entrypoint_codegen::facts::Severity;
using entrypoint_codegen::facts::SourceLocation;

TEST(EmitDiagnosticsTest, WritesCompilerStyleDiagnosticLine) {
  // Arrange
  EntrypointFacts facts;
  const SourceLocation location{
      .file = facts.add_source_file("entrypoint.cpp"),
      .line = 7,
      .column = 9,
  };
  facts.add_diagnostic(Diagnostic{
      .code = DiagnosticCode::kMissingNamePrefix,
      .message = std::nullopt,
      .severity = Severity::kError,
      .function = std::nullopt,
      .location = location,
  });
  std::ostringstream output;

  // Act
  entrypoint_codegen::emit::write_diagnostics(output, facts);

  // Assert
  EXPECT_EQ(output.str(), "entrypoint.cpp:7:9: error: entrypoint function must use prompp_ prefix [missing_name_prefix]\n");
}

}  // namespace
