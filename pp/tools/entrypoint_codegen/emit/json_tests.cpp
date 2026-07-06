#include "emit/json.h"

#include <gtest/gtest.h>

#include "diagnostics/diagnostics.h"

#include <optional>
#include <sstream>
#include <string>
#include <string_view>

namespace {

using entrypoint_codegen::diagnostics::Diagnostic;
using entrypoint_codegen::diagnostics::DiagnosticCode;
using entrypoint_codegen::diagnostics::DiagnosticSet;
using entrypoint_codegen::diagnostics::Severity;
using entrypoint_codegen::facts::BridgeKind;
using entrypoint_codegen::facts::EntrypointFacts;
using entrypoint_codegen::facts::FunctionDecl;
using entrypoint_codegen::facts::SourceLocation;

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

class EmitJsonTest : public testing::Test {
 protected:
  EntrypointFacts facts_;
  DiagnosticSet diagnostics_;
  std::ostringstream output_;
};

TEST_F(EmitJsonTest, WritesFunctionFacts) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 2, .column = 4};
  facts_.add_function(FunctionDecl{
      .name = facts_.add_string("prompp_store"),
      .return_type_spelling = facts_.add_string("void"),
      .documentation = facts_.add_string(""),
      .bridge_kind = BridgeKind::kCGo,
      .params = facts_.add_params({}),
      .layouts = facts_.add_layouts({}),
      .location = location,
      .has_c_linkage = true,
  });

  // Act
  entrypoint_codegen::emit::write_json(output_, facts_, diagnostics_);
  const std::string json = output_.str();
  const bool has_function_name = json.find("\"name\": \"prompp_store\"") != std::string::npos;
  const bool has_bridge_kind = json.find("\"bridge_kind\": \"cgo\"") != std::string::npos;
  const bool has_c_linkage = json.find("\"has_c_linkage\": true") != std::string::npos;

  // Assert
  EXPECT_TRUE(has_function_name);
  EXPECT_TRUE(has_bridge_kind);
  EXPECT_TRUE(has_c_linkage);
}

TEST_F(EmitJsonTest, EscapesStringFieldsInOutput) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entry\"point.cpp"), .line = 2, .column = 4};
  diagnostics_.add(Diagnostic{
      .code = DiagnosticCode::kClangDiagnostic,
      .message = "line\nmessage",
      .severity = Severity::kError,
      .function = std::nullopt,
      .location = location,
  });

  // Act
  entrypoint_codegen::emit::write_json(output_, facts_, diagnostics_);
  const std::string json = output_.str();
  const bool has_escaped_path = json.find("\"path\": \"entry\\\"point.cpp\"") != std::string::npos;
  const bool has_escaped_message = json.find("\"message\": \"line\\nmessage\"") != std::string::npos;

  // Assert
  EXPECT_TRUE(has_escaped_path);
  EXPECT_TRUE(has_escaped_message);
}

TEST_F(EmitJsonTest, EscapesNonWhitespaceControlCharactersInStringFields) {
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
      .function = std::nullopt,
      .location = std::nullopt,
  });

  // Act
  entrypoint_codegen::emit::write_json(output_, facts_, diagnostics_);
  const std::string json = output_.str();

  // Assert
  EXPECT_FALSE(has_unescaped_control_character_in_string(json));
  EXPECT_NE(json.find("\\u0001"), std::string::npos);
  EXPECT_NE(json.find("\\u0008"), std::string::npos);
  EXPECT_NE(json.find("\\u000c"), std::string::npos);
}

TEST_F(EmitJsonTest, WritesDiagnosticLocationObjectWhenPresent) {
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
  entrypoint_codegen::emit::write_json(output_, facts_, diagnostics_);
  const std::string json = output_.str();
  const bool has_location = json.find("\"location\": {\"file\": \"entrypoint.cpp\", \"line\": 7, \"column\": 9}") != std::string::npos;

  // Assert
  EXPECT_TRUE(has_location);
}

TEST_F(EmitJsonTest, WritesNullDiagnosticLocationWhenAbsent) {
  // Arrange
  diagnostics_.add(Diagnostic{
      .code = DiagnosticCode::kRuntimeMemoryUsage,
      .message = std::nullopt,
      .severity = Severity::kInfo,
      .function = std::nullopt,
      .location = std::nullopt,
  });

  // Act
  entrypoint_codegen::emit::write_json(output_, facts_, diagnostics_);
  const std::string json = output_.str();
  const bool has_null_location = json.find("\"location\": null") != std::string::npos;

  // Assert
  EXPECT_TRUE(has_null_location);
}

}  // namespace
