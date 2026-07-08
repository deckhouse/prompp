#include "contract/entrypoint_contract.h"

#include <gmock/gmock.h>
#include <gtest/gtest.h>

#include "diagnostics/diagnostics.h"
#include "facts/fact_arena.h"

#include <optional>
#include <vector>

namespace {

using epgen::diagnostics::DiagnosticCode;
using epgen::diagnostics::DiagnosticSet;
using epgen::diagnostics::Severity;
using epgen::facts::BridgeKind;
using epgen::facts::FactArena;
using epgen::facts::FunctionDecl;
using epgen::facts::LayoutDecl;
using epgen::facts::LayoutKind;
using epgen::facts::ParamDecl;
using epgen::facts::ParamRole;
using epgen::facts::SourceLocation;

TEST(EntrypointContractTest, RecognizesPromppFunctionPrefix) {
  // Arrange

  // Act
  const bool entrypoint_name = epgen::contract::is_entrypoint_function_name("prompp_fn");
  const bool other_name = epgen::contract::is_entrypoint_function_name("other_fn");

  // Assert
  EXPECT_TRUE(entrypoint_name);
  EXPECT_FALSE(other_name);
}

TEST(EntrypointContractTest, ClassifiesBridgeAnnotationNames) {
  // Arrange

  // Act
  const BridgeKind cgo = epgen::contract::bridge_kind_for_annotation("prompp.entrypoint.cgo");
  const BridgeKind fastcgo = epgen::contract::bridge_kind_for_annotation("prompp.entrypoint.fastcgo");
  const BridgeKind unknown = epgen::contract::bridge_kind_for_annotation("other");

  // Assert
  EXPECT_EQ(cgo, BridgeKind::kCGo);
  EXPECT_EQ(fastcgo, BridgeKind::kFastCGo);
  EXPECT_EQ(unknown, BridgeKind::kUnknown);
}

TEST(EntrypointContractTest, ClassifiesParameterNames) {
  // Arrange

  // Act
  const ParamRole args = epgen::contract::param_role_for_name("args");
  const ParamRole res = epgen::contract::param_role_for_name("res");
  const ParamRole other = epgen::contract::param_role_for_name("other");

  // Assert
  EXPECT_EQ(args, ParamRole::kArgs);
  EXPECT_EQ(res, ParamRole::kRes);
  EXPECT_EQ(other, ParamRole::kOther);
}

TEST(EntrypointContractTest, ClassifiesLayoutNames) {
  // Arrange

  // Act
  const std::optional<LayoutKind> arguments = epgen::contract::layout_kind_for_name("Arguments");
  const std::optional<LayoutKind> result = epgen::contract::layout_kind_for_name("Result");
  const std::optional<LayoutKind> other = epgen::contract::layout_kind_for_name("Other");

  // Assert
  ASSERT_TRUE(arguments.has_value());
  EXPECT_EQ(*arguments, LayoutKind::kArguments);
  ASSERT_TRUE(result.has_value());
  EXPECT_EQ(*result, LayoutKind::kResult);
  EXPECT_FALSE(other.has_value());
}

TEST(EntrypointContractTest, RecognizesVoidPointerSpellings) {
  // Arrange

  // Act
  const bool compact = epgen::contract::is_void_pointer_type("void*");
  const bool spaced = epgen::contract::is_void_pointer_type("void *");
  const bool const_pointer = epgen::contract::is_void_pointer_type("const void*");

  // Assert
  EXPECT_TRUE(compact);
  EXPECT_TRUE(spaced);
  EXPECT_FALSE(const_pointer);
}

class ValidateContractTest : public testing::Test {
 protected:
  FactArena facts_;
  DiagnosticSet diagnostics_;
};

TEST_F(ValidateContractTest, AcceptsValidCgoFunction) {
  // Function layout:
  //   extern C cgo void prompp_fn()
  //   layouts: none

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
  epgen::contract::validate_contract(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  EXPECT_TRUE(diagnostics.empty());
}

TEST_F(ValidateContractTest, AcceptsValidFastCgoFunction) {
  // Function layout:
  //   extern C fastcgo void prompp_fn(void* args, void* res)
  //   layouts: Arguments, Result

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
  epgen::contract::validate_contract(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  EXPECT_TRUE(diagnostics.empty());
}

TEST_F(ValidateContractTest, ReportsMissingEntrypointPrefix) {
  // Function layout:
  //   extern C cgo void other_fn()
  //   layouts: none

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
  epgen::contract::validate_contract(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kMissingNamePrefix);
  EXPECT_EQ(diagnostics[0].severity, Severity::kError);
  EXPECT_EQ(diagnostics[0].function, function_id);
  ASSERT_TRUE(diagnostics[0].location.has_value());
  EXPECT_EQ(diagnostics[0].location->line, location.line);
}

TEST_F(ValidateContractTest, ReportsMissingCLinkage) {
  // Function layout:
  //   C++ linkage cgo void prompp_fn()
  //   layouts: none

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
  epgen::contract::validate_contract(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kMissingCLinkage);
}

TEST_F(ValidateContractTest, ReportsMissingEntrypointAttribute) {
  // Function layout:
  //   extern C unknown-bridge void prompp_fn()
  //   layouts: none

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
  epgen::contract::validate_contract(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kMissingEntrypointAttribute);
}

TEST_F(ValidateContractTest, ReportsMultipleDiagnosticsForOneFunction) {
  // Function layout:
  //   C++ linkage fastcgo int other_fn(int* args)
  //   layouts: none

  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 7, .column = 3};
  const std::vector<ParamDecl> params{
      ParamDecl{.name = facts_.add_string("args"), .type_spelling = facts_.add_string("int*"), .role = ParamRole::kArgs, .location = location},
  };
  const auto function_id = facts_.add_function(FunctionDecl{
      .name = facts_.add_string("other_fn"),
      .return_type_spelling = facts_.add_string("int"),
      .documentation = facts_.add_string(""),
      .bridge_kind = BridgeKind::kFastCGo,
      .params = facts_.add_params(params),
      .layouts = facts_.add_layouts({}),
      .location = location,
      .has_c_linkage = false,
  });

  // Act
  epgen::contract::validate_contract(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  std::vector<DiagnosticCode> diagnostic_codes;
  std::vector<std::optional<epgen::facts::FunctionId>> diagnostic_functions;

  for (const auto& diagnostic : diagnostics) {
    diagnostic_codes.push_back(diagnostic.code);
    diagnostic_functions.push_back(diagnostic.function);
  }

  // Assert
  ASSERT_EQ(diagnostics.size(), 5);
  EXPECT_THAT(diagnostic_codes,
              testing::UnorderedElementsAre(DiagnosticCode::kMissingNamePrefix, DiagnosticCode::kMissingCLinkage, DiagnosticCode::kUnsupportedReturnType,
                                            DiagnosticCode::kUnsupportedParamType, DiagnosticCode::kMissingArgumentsLayout));
  EXPECT_THAT(diagnostic_functions, testing::Each(std::optional<epgen::facts::FunctionId>(function_id)));
}

TEST_F(ValidateContractTest, ReportsUnsupportedFastCgoReturnType) {
  // Function layout:
  //   extern C fastcgo int prompp_fn()
  //   layouts: none

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
  epgen::contract::validate_contract(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kUnsupportedReturnType);
}

TEST_F(ValidateContractTest, ReportsUnsupportedFastCgoParamCount) {
  // Function layout:
  //   extern C fastcgo void prompp_fn(void* args, void* res, void* res)
  //   layouts: Arguments, Result

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
  epgen::contract::validate_contract(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kUnsupportedParamCount);
}

TEST_F(ValidateContractTest, ReportsUnsupportedFastCgoParamType) {
  // Function layout:
  //   extern C fastcgo void prompp_fn(int* args)
  //   layouts: Arguments

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
  epgen::contract::validate_contract(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kUnsupportedParamType);
}

TEST_F(ValidateContractTest, ReportsUnknownFastCgoParamRole) {
  // Function layout:
  //   extern C fastcgo void prompp_fn(void* other)
  //   layouts: none

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
  epgen::contract::validate_contract(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kUnknownParamRole);
}

TEST_F(ValidateContractTest, ReportsInvalidTwoParamOrder) {
  // Function layout:
  //   extern C fastcgo void prompp_fn(void* res, void* res)
  //   layouts: Result

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
  epgen::contract::validate_contract(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kInvalidTwoParamOrder);
}

TEST_F(ValidateContractTest, ReportsInvalidSecondParamRole) {
  // Function layout:
  //   extern C fastcgo void prompp_fn(void* args, void* args)
  //   layouts: Arguments

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
  epgen::contract::validate_contract(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kInvalidSecondParamRole);
}

TEST_F(ValidateContractTest, ReportsMissingArgumentsLayout) {
  // Function layout:
  //   extern C fastcgo void prompp_fn(void* args)
  //   layouts: none

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
  epgen::contract::validate_contract(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kMissingArgumentsLayout);
}

TEST_F(ValidateContractTest, ReportsMissingResultLayout) {
  // Function layout:
  //   extern C fastcgo void prompp_fn(void* res)
  //   layouts: none

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
  epgen::contract::validate_contract(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kMissingResultLayout);
}

TEST_F(ValidateContractTest, ReportsUnexpectedArgumentsLayout) {
  // Function layout:
  //   extern C fastcgo void prompp_fn()
  //   layouts: Arguments

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
  epgen::contract::validate_contract(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kUnexpectedArgumentsLayout);
}

TEST_F(ValidateContractTest, ReportsUnexpectedResultLayout) {
  // Function layout:
  //   extern C fastcgo void prompp_fn()
  //   layouts: Result

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
  epgen::contract::validate_contract(facts_, diagnostics_);
  const auto diagnostics = diagnostics_.diagnostics();

  // Assert
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kUnexpectedResultLayout);
}

}  // namespace
