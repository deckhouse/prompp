#include "app/runtime_debug.h"

#include <gtest/gtest.h>

#include "diagnostics/diagnostics.h"

namespace {

using epgen::app::MemoryUsageSnapshot;
using epgen::diagnostics::DiagnosticCode;
using epgen::diagnostics::DiagnosticSet;
using epgen::diagnostics::Severity;

TEST(RuntimeDebugTest, AppendsMemoryUsageDiagnostic) {
  // Arrange
  DiagnosticSet diagnostics;

  // Act
  epgen::app::append_runtime_debug_diagnostics(diagnostics, MemoryUsageSnapshot{
                                                                .allocated_bytes = 11,
                                                                .deallocated_bytes = 7,
                                                                .peak_live_bytes = 9,
                                                            });
  const auto diagnostic_values = diagnostics.diagnostics();

  // Assert
  ASSERT_EQ(diagnostic_values.size(), 1);
  EXPECT_EQ(diagnostic_values[0].code, DiagnosticCode::kRuntimeMemoryUsage);
  EXPECT_EQ(diagnostic_values[0].severity, Severity::kInfo);
  ASSERT_TRUE(diagnostic_values[0].message.has_value());
  EXPECT_EQ(*diagnostic_values[0].message, "App PMR allocations: allocated=11 deallocated=7 peak_live=9 bytes");
  EXPECT_FALSE(diagnostic_values[0].location.has_value());
}

}  // namespace
