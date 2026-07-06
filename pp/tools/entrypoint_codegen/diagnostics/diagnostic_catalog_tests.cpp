#include "diagnostics/diagnostic_catalog.h"

#include <gtest/gtest.h>

#include "facts/facts.h"

#include <optional>
#include <string_view>

namespace {

using entrypoint_codegen::diagnostics::Diagnostic;
using entrypoint_codegen::diagnostics::DiagnosticCode;
using entrypoint_codegen::diagnostics::Severity;
using entrypoint_codegen::facts::SourceFileId;
using entrypoint_codegen::facts::SourceLocation;

TEST(DiagnosticCatalogTest, NamesDiagnosticCode) {
  // Act
  const std::string_view code = entrypoint_codegen::diagnostics::diagnostic_code_name(DiagnosticCode::kMissingNamePrefix);

  // Assert
  EXPECT_EQ(code, "missing_name_prefix");
}

TEST(DiagnosticCatalogTest, NamesParameterRoleDiagnosticsWithDistinctCodes) {
  // Act
  const std::string_view unknown_role = entrypoint_codegen::diagnostics::diagnostic_code_name(DiagnosticCode::kUnknownParamRole);
  const std::string_view invalid_order = entrypoint_codegen::diagnostics::diagnostic_code_name(DiagnosticCode::kInvalidTwoParamOrder);
  const std::string_view invalid_second = entrypoint_codegen::diagnostics::diagnostic_code_name(DiagnosticCode::kInvalidSecondParamRole);

  // Assert
  EXPECT_EQ(unknown_role, "unknown_param_role");
  EXPECT_EQ(invalid_order, "invalid_two_param_order");
  EXPECT_EQ(invalid_second, "invalid_second_param_role");
}

TEST(DiagnosticCatalogTest, ProvidesDefaultMessageForDiagnosticCode) {
  // Act
  const std::string_view message = entrypoint_codegen::diagnostics::diagnostic_default_message(DiagnosticCode::kMissingNamePrefix);

  // Assert
  EXPECT_EQ(message, "entrypoint function must use prompp_ prefix");
}

TEST(DiagnosticCatalogTest, UsesDiagnosticMessageWhenPresent) {
  // Arrange
  const SourceLocation location{.file = SourceFileId(0), .line = 1, .column = 1};
  const Diagnostic diagnostic{
      .code = DiagnosticCode::kClangDiagnostic,
      .message = "clang says no",
      .severity = Severity::kError,
      .function = std::nullopt,
      .location = location,
  };

  // Act
  const std::string_view message = entrypoint_codegen::diagnostics::diagnostic_message(diagnostic);

  // Assert
  EXPECT_EQ(message, "clang says no");
}

TEST(DiagnosticCatalogTest, FallsBackToDefaultMessageWhenDiagnosticMessageIsAbsent) {
  // Arrange
  const SourceLocation location{.file = SourceFileId(0), .line = 1, .column = 1};
  const Diagnostic diagnostic{
      .code = DiagnosticCode::kMissingNamePrefix,
      .message = std::nullopt,
      .severity = Severity::kError,
      .function = std::nullopt,
      .location = location,
  };

  // Act
  const std::string_view message = entrypoint_codegen::diagnostics::diagnostic_message(diagnostic);

  // Assert
  EXPECT_EQ(message, "entrypoint function must use prompp_ prefix");
}

TEST(DiagnosticCatalogTest, NamesSeverityValues) {
  // Act
  const std::string_view info = entrypoint_codegen::diagnostics::severity_name(Severity::kInfo);
  const std::string_view warning = entrypoint_codegen::diagnostics::severity_name(Severity::kWarning);
  const std::string_view error = entrypoint_codegen::diagnostics::severity_name(Severity::kError);

  // Assert
  EXPECT_EQ(info, "info");
  EXPECT_EQ(warning, "warning");
  EXPECT_EQ(error, "error");
}

}  // namespace
