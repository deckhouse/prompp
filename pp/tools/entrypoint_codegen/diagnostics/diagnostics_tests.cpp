#include "diagnostics/diagnostics.h"

#include <gtest/gtest.h>

#include "facts/fact_arena.h"

#include <string_view>

namespace {

using epgen::diagnostics::Diagnostic;
using epgen::diagnostics::DiagnosticCode;
using epgen::diagnostics::DiagnosticSet;
using epgen::diagnostics::Severity;
using epgen::facts::FactArena;
using epgen::facts::SourceLocation;

class DiagnosticSetTest : public testing::Test {
 protected:
  FactArena facts_;
  DiagnosticSet diagnostics_;
};

TEST_F(DiagnosticSetTest, StartsEmpty) {
  // Arrange

  // Act
  const bool empty = diagnostics_.empty();
  const uint32_t count = diagnostics_.count();

  // Assert
  EXPECT_TRUE(empty);
  EXPECT_EQ(count, 0);
}

TEST(DiagnosticsTest, DefaultDiagnosticUsesInvalidFactReferences) {
  // Arrange
  const Diagnostic diagnostic{
      .code = DiagnosticCode::kRuntimeMemoryUsage,
      .severity = Severity::kInfo,
  };

  // Act
  const bool message_is_valid = !diagnostic.message.empty();
  const bool function_is_valid = diagnostic.function.is_valid();
  const bool location_is_valid = diagnostic.location.is_valid();

  // Assert
  EXPECT_FALSE(message_is_valid);
  EXPECT_FALSE(function_is_valid);
  EXPECT_FALSE(location_is_valid);
}

TEST_F(DiagnosticSetTest, StoresDiagnosticsInInsertionOrder) {
  // Arrange
  const SourceLocation first_location{.file = facts_.add_source_file("first.cpp"), .line = 3, .column = 5};
  const SourceLocation second_location{.file = facts_.add_source_file("second.cpp"), .line = 8, .column = 13};

  // Act
  diagnostics_.add(Diagnostic{
      .code = DiagnosticCode::kMissingNamePrefix,
      .severity = Severity::kError,
      .location = first_location,
  });
  diagnostics_.add(Diagnostic{
      .code = DiagnosticCode::kRuntimeMemoryUsage,
      .message = facts_.add_string("memory diagnostic"),
      .severity = Severity::kInfo,
      .location = second_location,
  });
  const auto stored = diagnostics_.diagnostics();

  // Assert
  EXPECT_EQ(diagnostics_.count(), 2);
  ASSERT_EQ(stored.size(), 2);

  EXPECT_EQ(stored[0].code, DiagnosticCode::kMissingNamePrefix);
  ASSERT_TRUE(stored[0].location.is_valid());
  EXPECT_TRUE(stored[0].message.empty());
  EXPECT_EQ(stored[0].location.line, first_location.line);
  EXPECT_EQ(stored[0].location.column, first_location.column);

  EXPECT_EQ(stored[1].code, DiagnosticCode::kRuntimeMemoryUsage);
  ASSERT_FALSE(stored[1].message.empty());
  EXPECT_EQ(facts_.string(stored[1].message), "memory diagnostic");
  ASSERT_TRUE(stored[1].location.is_valid());
  EXPECT_EQ(stored[1].location.line, second_location.line);
  EXPECT_EQ(stored[1].location.column, second_location.column);
}

TEST_F(DiagnosticSetTest, CountsDiagnosticsBySeverity) {
  // Arrange
  diagnostics_.add(Diagnostic{
      .code = DiagnosticCode::kRuntimeMemoryUsage,
      .severity = Severity::kInfo,
  });
  diagnostics_.add(Diagnostic{
      .code = DiagnosticCode::kClangDiagnostic,
      .severity = Severity::kWarning,
  });
  diagnostics_.add(Diagnostic{
      .code = DiagnosticCode::kMissingNamePrefix,
      .severity = Severity::kError,
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

TEST(DiagnosticsTest, KnownDiagnosticCodeNameDoesNotUseUnknownFallback) {
  // Arrange
  constexpr DiagnosticCode code = DiagnosticCode::kMissingNamePrefix;

  // Act
  const std::string_view name = epgen::diagnostics::diagnostic_code_name(code);

  // Assert
  EXPECT_FALSE(name.empty());
  EXPECT_NE(name, "unknown_diagnostic");
}

TEST(DiagnosticsTest, UnknownDiagnosticCodeNameUsesUnknownFallback) {
  // Arrange
  const auto code = static_cast<DiagnosticCode>(255);

  // Act
  const std::string_view name = epgen::diagnostics::diagnostic_code_name(code);

  // Assert
  EXPECT_EQ(name, "unknown_diagnostic");
}

TEST(DiagnosticsTest, KnownDiagnosticCodeHasDefaultMessage) {
  // Arrange
  constexpr DiagnosticCode code = DiagnosticCode::kMissingNamePrefix;

  // Act
  const std::string_view message = epgen::diagnostics::diagnostic_default_message(code);

  // Assert
  EXPECT_FALSE(message.empty());
  EXPECT_NE(message, "unknown diagnostic");
}

TEST(DiagnosticsTest, UnknownDiagnosticCodeDefaultMessageUsesUnknownFallback) {
  // Arrange
  const auto code = static_cast<DiagnosticCode>(255);

  // Act
  const std::string_view message = epgen::diagnostics::diagnostic_default_message(code);

  // Assert
  EXPECT_EQ(message, "unknown diagnostic");
}

TEST(DiagnosticsTest, FallsBackToDefaultMessageWhenDiagnosticMessageIsAbsent) {
  // Arrange
  FactArena facts;
  const Diagnostic diagnostic{
      .code = DiagnosticCode::kMissingNamePrefix,
      .severity = Severity::kError,
  };

  // Act
  const std::string_view message = epgen::diagnostics::diagnostic_message(diagnostic);

  // Assert
  EXPECT_EQ(message, epgen::diagnostics::diagnostic_default_message(diagnostic.code));
}

TEST(DiagnosticsTest, UsesStoredDiagnosticMessageWhenPresent) {
  // Arrange
  FactArena facts;
  const Diagnostic diagnostic{
      .code = DiagnosticCode::kClangDiagnostic,
      .message = facts.add_string("clang says no"),
      .severity = Severity::kError,
  };

  // Act
  const std::string_view message = epgen::diagnostics::diagnostic_message(diagnostic);

  // Assert
  EXPECT_EQ(message, "clang says no");
}

TEST(DiagnosticsTest, NamesSeverityValues) {
  // Arrange
  constexpr Severity info_severity = Severity::kInfo;
  constexpr Severity warning_severity = Severity::kWarning;
  constexpr Severity error_severity = Severity::kError;

  // Act
  const std::string_view info = epgen::diagnostics::severity_name(info_severity);
  const std::string_view warning = epgen::diagnostics::severity_name(warning_severity);
  const std::string_view error = epgen::diagnostics::severity_name(error_severity);

  // Assert
  EXPECT_EQ(info, "info");
  EXPECT_EQ(warning, "warning");
  EXPECT_EQ(error, "error");
}

}  // namespace
