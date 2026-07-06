#include "diagnostics/diagnostics.h"

#include <gtest/gtest.h>

#include <optional>

namespace {

using entrypoint_codegen::diagnostics::Diagnostic;
using entrypoint_codegen::diagnostics::DiagnosticCode;
using entrypoint_codegen::diagnostics::DiagnosticSet;
using entrypoint_codegen::diagnostics::Severity;
using entrypoint_codegen::facts::SourceFileId;
using entrypoint_codegen::facts::SourceLocation;

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
  const entrypoint_codegen::diagnostics::SeverityCounts counts = entrypoint_codegen::diagnostics::count_by_severity(diagnostics_);

  // Assert
  EXPECT_EQ(counts.total, 3);
  EXPECT_EQ(counts.errors, 1);
  EXPECT_EQ(counts.warnings, 1);
  EXPECT_EQ(counts.infos, 1);
  EXPECT_TRUE(counts.has_errors());
}

}  // namespace
