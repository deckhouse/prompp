#include "diagnostics/diagnostics.h"

#include <gtest/gtest.h>

#include <optional>
#include <string_view>

namespace {

using epgen::diagnostics::Diagnostic;
using epgen::diagnostics::DiagnosticCode;
using epgen::diagnostics::DiagnosticSet;
using epgen::diagnostics::Severity;
using epgen::facts::SourceFileId;
using epgen::facts::SourceLocation;

class DiagnosticSetTest : public testing::Test {
 protected:
  DiagnosticSet diagnostics_;
};

TEST_F(DiagnosticSetTest, StartsEmpty) {
  EXPECT_TRUE(diagnostics_.empty());
  EXPECT_EQ(diagnostics_.count(), 0);
}

TEST_F(DiagnosticSetTest, StoresDiagnosticsAndCountsThem) {
  // Arrange
  const SourceLocation location{.file = SourceFileId(0), .line = 3, .column = 5};

  // Act
  diagnostics_.add(Diagnostic{
      .code = DiagnosticCode::kMissingNamePrefix,
      .message = std::nullopt,
      .severity = Severity::kError,
      .function = std::nullopt,
      .location = location,
  });
  const auto stored = diagnostics_.diagnostics();

  // Assert
  EXPECT_EQ(diagnostics_.count(), 1);
  ASSERT_EQ(stored.size(), 1);
  EXPECT_EQ(stored[0].code, DiagnosticCode::kMissingNamePrefix);
  ASSERT_TRUE(stored[0].location.has_value());
  EXPECT_EQ(stored[0].location->line, location.line);
  EXPECT_EQ(stored[0].location->column, location.column);
}

TEST_F(DiagnosticSetTest, CountsDiagnosticsBySeverity) {
  // Arrange
  diagnostics_.add(Diagnostic{
      .code = DiagnosticCode::kRuntimeMemoryUsage,
      .message = std::nullopt,
      .severity = Severity::kInfo,
      .function = std::nullopt,
      .location = std::nullopt,
  });
  diagnostics_.add(Diagnostic{
      .code = DiagnosticCode::kClangDiagnostic,
      .message = std::nullopt,
      .severity = Severity::kWarning,
      .function = std::nullopt,
      .location = std::nullopt,
  });
  diagnostics_.add(Diagnostic{
      .code = DiagnosticCode::kMissingNamePrefix,
      .message = std::nullopt,
      .severity = Severity::kError,
      .function = std::nullopt,
      .location = std::nullopt,
  });

  // Act
  const epgen::diagnostics::SeverityCounts counts = epgen::diagnostics::count_by_severity(diagnostics_);

  // Assert
  EXPECT_EQ(counts.total, 3);
  EXPECT_EQ(counts.errors, 1);
  EXPECT_EQ(counts.warnings, 1);
  EXPECT_EQ(counts.infos, 1);
  EXPECT_TRUE(counts.has_errors());
}

TEST(DiagnosticsTest, NamesDiagnosticCode) {
  // Act
  const std::string_view code = epgen::diagnostics::diagnostic_code_name(DiagnosticCode::kMissingNamePrefix);

  // Assert
  EXPECT_EQ(code, "missing_name_prefix");
}

TEST(DiagnosticsTest, NamesParameterRoleDiagnosticsWithDistinctCodes) {
  // Act
  const std::string_view unknown_role = epgen::diagnostics::diagnostic_code_name(DiagnosticCode::kUnknownParamRole);
  const std::string_view invalid_order = epgen::diagnostics::diagnostic_code_name(DiagnosticCode::kInvalidTwoParamOrder);
  const std::string_view invalid_second = epgen::diagnostics::diagnostic_code_name(DiagnosticCode::kInvalidSecondParamRole);

  // Assert
  EXPECT_EQ(unknown_role, "unknown_param_role");
  EXPECT_EQ(invalid_order, "invalid_two_param_order");
  EXPECT_EQ(invalid_second, "invalid_second_param_role");
}

TEST(DiagnosticsTest, ProvidesDefaultMessageForDiagnosticCode) {
  // Act
  const std::string_view message = epgen::diagnostics::diagnostic_default_message(DiagnosticCode::kMissingNamePrefix);

  // Assert
  EXPECT_EQ(message, "entrypoint function must use prompp_ prefix");
}

TEST(DiagnosticsTest, UsesDiagnosticMessageWhenPresent) {
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
  const std::string_view message = epgen::diagnostics::diagnostic_message(diagnostic);

  // Assert
  EXPECT_EQ(message, "clang says no");
}

TEST(DiagnosticsTest, FallsBackToDefaultMessageWhenDiagnosticMessageIsAbsent) {
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
  const std::string_view message = epgen::diagnostics::diagnostic_message(diagnostic);

  // Assert
  EXPECT_EQ(message, "entrypoint function must use prompp_ prefix");
}

TEST(DiagnosticsTest, NamesSeverityValues) {
  // Act
  const std::string_view info = epgen::diagnostics::severity_name(Severity::kInfo);
  const std::string_view warning = epgen::diagnostics::severity_name(Severity::kWarning);
  const std::string_view error = epgen::diagnostics::severity_name(Severity::kError);

  // Assert
  EXPECT_EQ(info, "info");
  EXPECT_EQ(warning, "warning");
  EXPECT_EQ(error, "error");
}

}  // namespace
