#include "app/runtime_debug.h"

#include <gtest/gtest.h>

#include "diagnostics/diagnostics.h"

#include <optional>

namespace {

using epgen::app::MemoryUsageSnapshot;
using epgen::diagnostics::DiagnosticCode;
using epgen::diagnostics::DiagnosticSet;
using epgen::diagnostics::Severity;

class RuntimeDebugTest : public testing::Test {
 protected:
  DiagnosticSet diagnostics_;
};

TEST_F(RuntimeDebugTest, AppendsMemoryUsageDiagnostic) {
  // Act
  epgen::app::append_runtime_debug_diagnostics(diagnostics_, MemoryUsageSnapshot{
                                                                 .allocated_bytes = 11,
                                                                 .deallocated_bytes = 7,
                                                                 .peak_live_bytes = 9,
                                                             });
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kRuntimeMemoryUsage);
  EXPECT_EQ(diagnostics[0].severity, Severity::kInfo);
  ASSERT_TRUE(diagnostics[0].message.has_value());
  EXPECT_EQ(*diagnostics[0].message, "App PMR allocations: allocated=11 deallocated=7 peak_live=9 bytes");
  EXPECT_FALSE(diagnostics[0].location.has_value());
}

}  // namespace
