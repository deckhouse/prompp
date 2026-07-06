#include "emit/diagnostics.h"

#include <gtest/gtest.h>

#include "diagnostics/diagnostics.h"

#include <optional>
#include <sstream>

namespace {

using entrypoint_codegen::diagnostics::Diagnostic;
using entrypoint_codegen::diagnostics::DiagnosticCode;
using entrypoint_codegen::diagnostics::DiagnosticSet;
using entrypoint_codegen::diagnostics::Severity;
using entrypoint_codegen::facts::EntrypointFacts;
using entrypoint_codegen::facts::SourceLocation;

class EmitDiagnosticsTest : public testing::Test {
 protected:
  EntrypointFacts facts_;
  DiagnosticSet diagnostics_;
  std::ostringstream output_;
};

TEST_F(EmitDiagnosticsTest, WritesCompilerStyleDiagnosticLine) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 7, .column = 9};
  diagnostics_.add(Diagnostic{
      .code = DiagnosticCode::kMissingNamePrefix,
      .message = std::nullopt,
      .severity = Severity::kError,
      .function = std::nullopt,
      .location = location,
  });

  // Act
  entrypoint_codegen::emit::write_diagnostics(output_, facts_, diagnostics_);

  // Assert
  EXPECT_EQ(output_.str(), "entrypoint.cpp:7:9: error: entrypoint function must use prompp_ prefix [missing_name_prefix]\n");
}

TEST_F(EmitDiagnosticsTest, WritesLocationlessDiagnosticLine) {
  // Arrange
  diagnostics_.add(Diagnostic{
      .code = DiagnosticCode::kRuntimeMemoryUsage,
      .message = std::nullopt,
      .severity = Severity::kInfo,
      .function = std::nullopt,
      .location = std::nullopt,
  });

  // Act
  entrypoint_codegen::emit::write_diagnostics(output_, facts_, diagnostics_);

  // Assert
  EXPECT_EQ(output_.str(), "info: runtime memory usage [runtime_memory_usage]\n");
}

}  // namespace
