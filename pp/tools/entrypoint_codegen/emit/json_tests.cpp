#include "emit/json.h"

#include <gtest/gtest.h>

#include <optional>
#include <sstream>
#include <string>

namespace {

using entrypoint_codegen::facts::Diagnostic;
using entrypoint_codegen::facts::DiagnosticCode;
using entrypoint_codegen::facts::EntrypointFacts;
using entrypoint_codegen::facts::Severity;
using entrypoint_codegen::facts::SourceLocation;

TEST(EmitJsonTest, EscapesStringFieldsInOutput) {
  // Arrange
  EntrypointFacts facts;
  const SourceLocation location{
      .file = facts.add_source_file("entry\"point.cpp"),
      .line = 2,
      .column = 4,
  };
  facts.add_diagnostic(Diagnostic{
      .code = DiagnosticCode::kClangDiagnostic,
      .message = facts.add_string("line\nmessage"),
      .severity = Severity::kError,
      .function = std::nullopt,
      .location = location,
  });
  std::ostringstream output;

  // Act
  entrypoint_codegen::emit::write_json(output, facts);

  // Assert
  const std::string json = output.str();
  EXPECT_NE(json.find("\"path\": \"entry\\\"point.cpp\""), std::string::npos);
  EXPECT_NE(json.find("\"message\": \"line\\nmessage\""), std::string::npos);
}

}  // namespace
