#include "emit/diagnostic_text.h"

#include <gtest/gtest.h>

#include <optional>

namespace {

using entrypoint_codegen::facts::Diagnostic;
using entrypoint_codegen::facts::DiagnosticCode;
using entrypoint_codegen::facts::EntrypointFacts;
using entrypoint_codegen::facts::Severity;
using entrypoint_codegen::facts::SourceLocation;

SourceLocation add_source_file(EntrypointFacts& facts) {
  return SourceLocation{
      .file = facts.add_source_file("entrypoint.cpp"),
      .line = 1,
      .column = 1,
  };
}

TEST(DiagnosticTextTest, ReturnsStaticMessageForValidationCode) {
  // Arrange
  EntrypointFacts facts;
  const Diagnostic diagnostic{
      .code = DiagnosticCode::kMissingNamePrefix,
      .message = std::nullopt,
      .severity = Severity::kError,
      .function = std::nullopt,
      .location = add_source_file(facts),
  };

  // Act
  const std::string_view code = entrypoint_codegen::emit::diagnostic_code_name(diagnostic.code);
  const std::string_view message = entrypoint_codegen::emit::diagnostic_message(facts, diagnostic);

  // Assert
  EXPECT_EQ(code, "missing_name_prefix");
  EXPECT_EQ(message, "entrypoint function must use prompp_ prefix");
}

TEST(DiagnosticTextTest, ReturnsDynamicMessageWhenPresent) {
  // Arrange
  EntrypointFacts facts;
  const Diagnostic diagnostic{
      .code = DiagnosticCode::kClangDiagnostic,
      .message = facts.add_string("clang says no"),
      .severity = Severity::kError,
      .function = std::nullopt,
      .location = add_source_file(facts),
  };

  // Act
  const std::string_view message = entrypoint_codegen::emit::diagnostic_message(facts, diagnostic);

  // Assert
  EXPECT_EQ(message, "clang says no");
}

}  // namespace
