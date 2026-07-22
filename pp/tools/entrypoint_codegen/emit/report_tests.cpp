#include "emit/report.h"

#include <gtest/gtest.h>

#include "diagnostics/diagnostics.h"

#include <sstream>
#include <string>
#include <string_view>

namespace {

using epgen::diagnostics::Diagnostic;
using epgen::diagnostics::DiagnosticCode;
using epgen::diagnostics::DiagnosticSet;
using epgen::diagnostics::Severity;
using epgen::facts::BridgeKind;
using epgen::facts::FactStore;
using epgen::facts::FunctionDecl;
using epgen::facts::SourceLocation;

bool has_unescaped_control_character_in_string(std::string_view json) {
  bool in_string = false;
  bool escaped = false;
  for (const unsigned char ch : json) {
    if (!in_string) {
      if (ch == '"') {
        in_string = true;
      }
      continue;
    }

    if (escaped) {
      escaped = false;
      continue;
    }
    if (ch == '\\') {
      escaped = true;
      continue;
    }
    if (ch == '"') {
      in_string = false;
      continue;
    }
    if (ch < 0x20) {
      return true;
    }
  }
  return false;
}

class EmitReportTest : public testing::Test {
 protected:
  FactStore facts_;
  DiagnosticSet diagnostics_;
  std::ostringstream output_;
};

TEST_F(EmitReportTest, WritesJsonFunctionFacts) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 2, .column = 4};
  facts_.add_function(FunctionDecl{
      .name = "prompp_store",
      .return_type_spelling = "void",
      .documentation = "",
      .bridge_kind = BridgeKind::kCGo,
      .params = {},
      .layouts = {},
      .location = location,
      .has_c_linkage = true,
  });

  // Act
  epgen::emit::write_report(output_, epgen::emit::ReportFormat::kJson, facts_, diagnostics_);
  const std::string json = output_.str();
  constexpr std::string_view expected =
      "{\n"
      "  \"source_files\": [\n"
      "    {\"path\": \"entrypoint.cpp\"}\n"
      "  ],\n"
      "  \"functions\": [\n"
      "    {\n"
      "      \"name\": \"prompp_store\",\n"
      "      \"return_type\": \"void\",\n"
      "      \"bridge_kind\": \"cgo\",\n"
      "      \"has_c_linkage\": true,\n"
      "      \"documentation\": \"\",\n"
      "      \"location\": {\"file\": \"entrypoint.cpp\", \"line\": 2, \"column\": 4},\n"
      "      \"params\": [],\n"
      "      \"layouts\": []\n"
      "    }\n"
      "  ],\n"
      "  \"diagnostics\": [\n"
      "  ]\n"
      "}\n";

  // Assert
  EXPECT_EQ(expected, json);
}

TEST_F(EmitReportTest, EscapesJsonStringFieldsInOutput) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entry\"point.cpp"), .line = 2, .column = 4};
  diagnostics_.add(Diagnostic{
      .code = DiagnosticCode::kClangDiagnostic,
      .message = "line\nmessage",
      .severity = Severity::kError,
      .location = location,
  });

  // Act
  epgen::emit::write_report(output_, epgen::emit::ReportFormat::kJson, facts_, diagnostics_);
  const std::string json = output_.str();
  const bool has_escaped_path = json.find("\"path\": \"entry\\\"point.cpp\"") != std::string::npos;
  const bool has_escaped_message = json.find("\"message\": \"line\\nmessage\"") != std::string::npos;

  // Assert
  EXPECT_TRUE(has_escaped_path);
  EXPECT_TRUE(has_escaped_message);
}

TEST_F(EmitReportTest, EscapesNonWhitespaceControlCharactersInJsonStringFields) {
  // Arrange
  std::string message = "before";
  message.push_back('\x01');
  message.push_back('\b');
  message.push_back('\f');
  message += "after";
  diagnostics_.add(Diagnostic{
      .code = DiagnosticCode::kClangDiagnostic,
      .message = message,
      .severity = Severity::kError,
  });

  // Act
  epgen::emit::write_report(output_, epgen::emit::ReportFormat::kJson, facts_, diagnostics_);
  const std::string json = output_.str();

  // Assert
  EXPECT_FALSE(has_unescaped_control_character_in_string(json));
  EXPECT_NE(json.find("\\u0001"), std::string::npos);
  EXPECT_NE(json.find("\\u0008"), std::string::npos);
  EXPECT_NE(json.find("\\u000c"), std::string::npos);
}

TEST_F(EmitReportTest, WritesJsonDiagnosticLocationObjectWhenPresent) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 7, .column = 9};
  diagnostics_.add(Diagnostic{
      .code = DiagnosticCode::kMissingNamePrefix,
      .severity = Severity::kError,
      .location = location,
  });

  // Act
  epgen::emit::write_report(output_, epgen::emit::ReportFormat::kJson, facts_, diagnostics_);
  const std::string json = output_.str();
  const bool has_location = json.find("\"location\": {\"file\": \"entrypoint.cpp\", \"line\": 7, \"column\": 9}") != std::string::npos;

  // Assert
  EXPECT_TRUE(has_location);
}

TEST_F(EmitReportTest, WritesInvalidJsonLocationFileWhenSourceFileIsAbsent) {
  // Arrange
  const SourceLocation location{.file = epgen::facts::SourceFileId{}, .line = 7, .column = 9};
  facts_.add_function(FunctionDecl{
      .name = "prompp_store",
      .return_type_spelling = "void",
      .documentation = "",
      .bridge_kind = BridgeKind::kCGo,
      .params = {},
      .layouts = {},
      .location = location,
      .has_c_linkage = true,
  });

  // Act
  epgen::emit::write_report(output_, epgen::emit::ReportFormat::kJson, facts_, diagnostics_);
  const std::string json = output_.str();
  const bool has_location = json.find("\"location\": {\"file\": \"<invalid>\", \"line\": 7, \"column\": 9}") != std::string::npos;

  // Assert
  EXPECT_TRUE(has_location);
}

TEST_F(EmitReportTest, WritesNullJsonDiagnosticLocationWhenAbsent) {
  // Arrange
  diagnostics_.add(Diagnostic{
      .code = DiagnosticCode::kClangDiagnostic,
      .severity = Severity::kInfo,
  });

  // Act
  epgen::emit::write_report(output_, epgen::emit::ReportFormat::kJson, facts_, diagnostics_);
  const std::string json = output_.str();
  const bool has_null_location = json.find("\"location\": null") != std::string::npos;

  // Assert
  EXPECT_TRUE(has_null_location);
}

TEST_F(EmitReportTest, WritesCompilerStyleDiagnosticLine) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 7, .column = 9};
  diagnostics_.add(Diagnostic{
      .code = DiagnosticCode::kMissingNamePrefix,
      .severity = Severity::kError,
      .location = location,
  });

  // Act
  epgen::emit::write_report(output_, epgen::emit::ReportFormat::kCompilerDiagnostics, facts_, diagnostics_);

  // Assert
  EXPECT_EQ(output_.str(), "entrypoint.cpp:7:9: error: entrypoint function must use prompp_ prefix [missing_name_prefix]\n");
}

TEST_F(EmitReportTest, WritesLocationlessDiagnosticLine) {
  // Arrange
  diagnostics_.add(Diagnostic{
      .code = DiagnosticCode::kClangDiagnostic,
      .severity = Severity::kInfo,
  });

  // Act
  epgen::emit::write_report(output_, epgen::emit::ReportFormat::kCompilerDiagnostics, facts_, diagnostics_);

  // Assert
  EXPECT_EQ(output_.str(), "info: Clang diagnostic [clang_diagnostic]\n");
}

}  // namespace
