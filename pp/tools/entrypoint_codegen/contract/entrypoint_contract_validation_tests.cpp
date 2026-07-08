#include "contract/entrypoint_contract.h"

#include <gtest/gtest.h>

#include "diagnostics/diagnostics.h"
#include "facts/entrypoint_facts.h"

#include <optional>
#include <vector>

namespace {

using entrypoint_codegen::diagnostics::DiagnosticCode;
using entrypoint_codegen::diagnostics::DiagnosticSet;
using entrypoint_codegen::diagnostics::Severity;
using entrypoint_codegen::facts::BridgeKind;
using entrypoint_codegen::facts::EntrypointFacts;
using entrypoint_codegen::facts::FunctionDecl;
using entrypoint_codegen::facts::LayoutDecl;
using entrypoint_codegen::facts::LayoutKind;
using entrypoint_codegen::facts::ParamDecl;
using entrypoint_codegen::facts::ParamRole;
using entrypoint_codegen::facts::SourceLocation;

class ValidateEntrypointsTest : public testing::Test {
 protected:
  EntrypointFacts facts_;
  DiagnosticSet diagnostics_;
};

TEST_F(ValidateEntrypointsTest, AcceptsValidCgoFunction) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 7, .column = 3};
  facts_.add_function(FunctionDecl{
      .name = facts_.add_string("prompp_fn"),
      .return_type_spelling = facts_.add_string("void"),
      .documentation = facts_.add_string(""),
      .bridge_kind = BridgeKind::kCGo,
      .params = facts_.add_params({}),
      .layouts = facts_.add_layouts({}),
      .location = location,
      .has_c_linkage = true,
  });

  // Act
  entrypoint_codegen::contract::validate_entrypoints(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  EXPECT_TRUE(diagnostics.empty());
}

TEST_F(ValidateEntrypointsTest, AcceptsValidFastCgoFunction) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 7, .column = 3};
  const std::vector<ParamDecl> params{
      ParamDecl{.name = facts_.add_string("args"), .type_spelling = facts_.add_string("void*"), .role = ParamRole::kArgs, .location = location},
      ParamDecl{.name = facts_.add_string("res"), .type_spelling = facts_.add_string("void*"), .role = ParamRole::kRes, .location = location},
  };
  const std::vector<LayoutDecl> layouts{
      LayoutDecl{.kind = LayoutKind::kArguments, .fields = facts_.add_fields({}), .location = location},
      LayoutDecl{.kind = LayoutKind::kResult, .fields = facts_.add_fields({}), .location = location},
  };
  facts_.add_function(FunctionDecl{
      .name = facts_.add_string("prompp_fn"),
      .return_type_spelling = facts_.add_string("void"),
      .documentation = facts_.add_string(""),
      .bridge_kind = BridgeKind::kFastCGo,
      .params = facts_.add_params(params),
      .layouts = facts_.add_layouts(layouts),
      .location = location,
      .has_c_linkage = true,
  });

  // Act
  entrypoint_codegen::contract::validate_entrypoints(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  EXPECT_TRUE(diagnostics.empty());
}

TEST_F(ValidateEntrypointsTest, ReportsMissingEntrypointPrefix) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 7, .column = 3};
  const auto function_id = facts_.add_function(FunctionDecl{
      .name = facts_.add_string("other_fn"),
      .return_type_spelling = facts_.add_string("void"),
      .documentation = facts_.add_string(""),
      .bridge_kind = BridgeKind::kCGo,
      .params = facts_.add_params({}),
      .layouts = facts_.add_layouts({}),
      .location = location,
      .has_c_linkage = true,
  });

  // Act
  entrypoint_codegen::contract::validate_entrypoints(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kMissingNamePrefix);
  EXPECT_EQ(diagnostics[0].severity, Severity::kError);
  EXPECT_EQ(diagnostics[0].function, function_id);
  ASSERT_TRUE(diagnostics[0].location.has_value());
  EXPECT_EQ(diagnostics[0].location->line, location.line);
}

TEST_F(ValidateEntrypointsTest, ReportsMissingCLinkage) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 7, .column = 3};
  facts_.add_function(FunctionDecl{
      .name = facts_.add_string("prompp_fn"),
      .return_type_spelling = facts_.add_string("void"),
      .documentation = facts_.add_string(""),
      .bridge_kind = BridgeKind::kCGo,
      .params = facts_.add_params({}),
      .layouts = facts_.add_layouts({}),
      .location = location,
      .has_c_linkage = false,
  });

  // Act
  entrypoint_codegen::contract::validate_entrypoints(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kMissingCLinkage);
}

TEST_F(ValidateEntrypointsTest, ReportsMissingEntrypointAttribute) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 7, .column = 3};
  facts_.add_function(FunctionDecl{
      .name = facts_.add_string("prompp_fn"),
      .return_type_spelling = facts_.add_string("void"),
      .documentation = facts_.add_string(""),
      .bridge_kind = BridgeKind::kUnknown,
      .params = facts_.add_params({}),
      .layouts = facts_.add_layouts({}),
      .location = location,
      .has_c_linkage = true,
  });

  // Act
  entrypoint_codegen::contract::validate_entrypoints(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kMissingEntrypointAttribute);
}

TEST_F(ValidateEntrypointsTest, ReportsUnsupportedFastCgoReturnType) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 7, .column = 3};
  facts_.add_function(FunctionDecl{
      .name = facts_.add_string("prompp_fn"),
      .return_type_spelling = facts_.add_string("int"),
      .documentation = facts_.add_string(""),
      .bridge_kind = BridgeKind::kFastCGo,
      .params = facts_.add_params({}),
      .layouts = facts_.add_layouts({}),
      .location = location,
      .has_c_linkage = true,
  });

  // Act
  entrypoint_codegen::contract::validate_entrypoints(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kUnsupportedReturnType);
}

TEST_F(ValidateEntrypointsTest, ReportsUnsupportedFastCgoParamCount) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 7, .column = 3};
  const std::vector<ParamDecl> params{
      ParamDecl{.name = facts_.add_string("args"), .type_spelling = facts_.add_string("void*"), .role = ParamRole::kArgs, .location = location},
      ParamDecl{.name = facts_.add_string("res"), .type_spelling = facts_.add_string("void*"), .role = ParamRole::kRes, .location = location},
      ParamDecl{.name = facts_.add_string("res"), .type_spelling = facts_.add_string("void*"), .role = ParamRole::kRes, .location = location},
  };
  const std::vector<LayoutDecl> layouts{
      LayoutDecl{.kind = LayoutKind::kArguments, .fields = facts_.add_fields({}), .location = location},
      LayoutDecl{.kind = LayoutKind::kResult, .fields = facts_.add_fields({}), .location = location},
  };
  facts_.add_function(FunctionDecl{
      .name = facts_.add_string("prompp_fn"),
      .return_type_spelling = facts_.add_string("void"),
      .documentation = facts_.add_string(""),
      .bridge_kind = BridgeKind::kFastCGo,
      .params = facts_.add_params(params),
      .layouts = facts_.add_layouts(layouts),
      .location = location,
      .has_c_linkage = true,
  });

  // Act
  entrypoint_codegen::contract::validate_entrypoints(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kUnsupportedParamCount);
}

TEST_F(ValidateEntrypointsTest, ReportsUnsupportedFastCgoParamType) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 7, .column = 3};
  const std::vector<ParamDecl> params{
      ParamDecl{.name = facts_.add_string("args"), .type_spelling = facts_.add_string("int*"), .role = ParamRole::kArgs, .location = location},
  };
  const std::vector<LayoutDecl> layouts{
      LayoutDecl{.kind = LayoutKind::kArguments, .fields = facts_.add_fields({}), .location = location},
  };
  facts_.add_function(FunctionDecl{
      .name = facts_.add_string("prompp_fn"),
      .return_type_spelling = facts_.add_string("void"),
      .documentation = facts_.add_string(""),
      .bridge_kind = BridgeKind::kFastCGo,
      .params = facts_.add_params(params),
      .layouts = facts_.add_layouts(layouts),
      .location = location,
      .has_c_linkage = true,
  });

  // Act
  entrypoint_codegen::contract::validate_entrypoints(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kUnsupportedParamType);
}

TEST_F(ValidateEntrypointsTest, ReportsUnknownFastCgoParamRole) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 7, .column = 3};
  const std::vector<ParamDecl> params{
      ParamDecl{.name = facts_.add_string("other"), .type_spelling = facts_.add_string("void*"), .role = ParamRole::kOther, .location = location},
  };
  facts_.add_function(FunctionDecl{
      .name = facts_.add_string("prompp_fn"),
      .return_type_spelling = facts_.add_string("void"),
      .documentation = facts_.add_string(""),
      .bridge_kind = BridgeKind::kFastCGo,
      .params = facts_.add_params(params),
      .layouts = facts_.add_layouts({}),
      .location = location,
      .has_c_linkage = true,
  });

  // Act
  entrypoint_codegen::contract::validate_entrypoints(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kUnknownParamRole);
}

TEST_F(ValidateEntrypointsTest, ReportsInvalidTwoParamOrder) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 7, .column = 3};
  const std::vector<ParamDecl> params{
      ParamDecl{.name = facts_.add_string("res"), .type_spelling = facts_.add_string("void*"), .role = ParamRole::kRes, .location = location},
      ParamDecl{.name = facts_.add_string("res"), .type_spelling = facts_.add_string("void*"), .role = ParamRole::kRes, .location = location},
  };
  const std::vector<LayoutDecl> layouts{
      LayoutDecl{.kind = LayoutKind::kResult, .fields = facts_.add_fields({}), .location = location},
  };
  facts_.add_function(FunctionDecl{
      .name = facts_.add_string("prompp_fn"),
      .return_type_spelling = facts_.add_string("void"),
      .documentation = facts_.add_string(""),
      .bridge_kind = BridgeKind::kFastCGo,
      .params = facts_.add_params(params),
      .layouts = facts_.add_layouts(layouts),
      .location = location,
      .has_c_linkage = true,
  });

  // Act
  entrypoint_codegen::contract::validate_entrypoints(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kInvalidTwoParamOrder);
}

TEST_F(ValidateEntrypointsTest, ReportsInvalidSecondParamRole) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 7, .column = 3};
  const std::vector<ParamDecl> params{
      ParamDecl{.name = facts_.add_string("args"), .type_spelling = facts_.add_string("void*"), .role = ParamRole::kArgs, .location = location},
      ParamDecl{.name = facts_.add_string("args"), .type_spelling = facts_.add_string("void*"), .role = ParamRole::kArgs, .location = location},
  };
  const std::vector<LayoutDecl> layouts{
      LayoutDecl{.kind = LayoutKind::kArguments, .fields = facts_.add_fields({}), .location = location},
  };
  facts_.add_function(FunctionDecl{
      .name = facts_.add_string("prompp_fn"),
      .return_type_spelling = facts_.add_string("void"),
      .documentation = facts_.add_string(""),
      .bridge_kind = BridgeKind::kFastCGo,
      .params = facts_.add_params(params),
      .layouts = facts_.add_layouts(layouts),
      .location = location,
      .has_c_linkage = true,
  });

  // Act
  entrypoint_codegen::contract::validate_entrypoints(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kInvalidSecondParamRole);
}

TEST_F(ValidateEntrypointsTest, ReportsMissingArgumentsLayout) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 7, .column = 3};
  const std::vector<ParamDecl> params{
      ParamDecl{.name = facts_.add_string("args"), .type_spelling = facts_.add_string("void*"), .role = ParamRole::kArgs, .location = location},
  };
  facts_.add_function(FunctionDecl{
      .name = facts_.add_string("prompp_fn"),
      .return_type_spelling = facts_.add_string("void"),
      .documentation = facts_.add_string(""),
      .bridge_kind = BridgeKind::kFastCGo,
      .params = facts_.add_params(params),
      .layouts = facts_.add_layouts({}),
      .location = location,
      .has_c_linkage = true,
  });

  // Act
  entrypoint_codegen::contract::validate_entrypoints(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kMissingArgumentsLayout);
}

TEST_F(ValidateEntrypointsTest, ReportsMissingResultLayout) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 7, .column = 3};
  const std::vector<ParamDecl> params{
      ParamDecl{.name = facts_.add_string("res"), .type_spelling = facts_.add_string("void*"), .role = ParamRole::kRes, .location = location},
  };
  facts_.add_function(FunctionDecl{
      .name = facts_.add_string("prompp_fn"),
      .return_type_spelling = facts_.add_string("void"),
      .documentation = facts_.add_string(""),
      .bridge_kind = BridgeKind::kFastCGo,
      .params = facts_.add_params(params),
      .layouts = facts_.add_layouts({}),
      .location = location,
      .has_c_linkage = true,
  });

  // Act
  entrypoint_codegen::contract::validate_entrypoints(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kMissingResultLayout);
}

TEST_F(ValidateEntrypointsTest, ReportsUnexpectedArgumentsLayout) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 7, .column = 3};
  const std::vector<LayoutDecl> layouts{
      LayoutDecl{.kind = LayoutKind::kArguments, .fields = facts_.add_fields({}), .location = location},
  };
  facts_.add_function(FunctionDecl{
      .name = facts_.add_string("prompp_fn"),
      .return_type_spelling = facts_.add_string("void"),
      .documentation = facts_.add_string(""),
      .bridge_kind = BridgeKind::kFastCGo,
      .params = facts_.add_params({}),
      .layouts = facts_.add_layouts(layouts),
      .location = location,
      .has_c_linkage = true,
  });

  // Act
  entrypoint_codegen::contract::validate_entrypoints(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kUnexpectedArgumentsLayout);
}

TEST_F(ValidateEntrypointsTest, ReportsUnexpectedResultLayout) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 7, .column = 3};
  const std::vector<LayoutDecl> layouts{
      LayoutDecl{.kind = LayoutKind::kResult, .fields = facts_.add_fields({}), .location = location},
  };
  facts_.add_function(FunctionDecl{
      .name = facts_.add_string("prompp_fn"),
      .return_type_spelling = facts_.add_string("void"),
      .documentation = facts_.add_string(""),
      .bridge_kind = BridgeKind::kFastCGo,
      .params = facts_.add_params({}),
      .layouts = facts_.add_layouts(layouts),
      .location = location,
      .has_c_linkage = true,
  });

  // Act
  entrypoint_codegen::contract::validate_entrypoints(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kUnexpectedResultLayout);
}

}  // namespace
