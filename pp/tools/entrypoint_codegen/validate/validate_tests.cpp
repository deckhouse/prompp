#include "validate/validate.h"

#include <gtest/gtest.h>

#include <span>
#include <vector>

namespace {

using entrypoint_codegen::facts::BridgeKind;
using entrypoint_codegen::facts::DiagnosticCode;
using entrypoint_codegen::facts::EntrypointFacts;
using entrypoint_codegen::facts::FunctionDecl;
using entrypoint_codegen::facts::LayoutDecl;
using entrypoint_codegen::facts::ParamDecl;
using entrypoint_codegen::facts::ParamRole;
using entrypoint_codegen::facts::Severity;
using entrypoint_codegen::facts::SourceLocation;

SourceLocation add_source_file(EntrypointFacts& facts) {
  return SourceLocation{
      .file = facts.add_source_file("entrypoint.cpp"),
      .line = 7,
      .column = 3,
  };
}

entrypoint_codegen::facts::ParamRange add_params(EntrypointFacts& facts, std::span<const ParamDecl> params) {
  return facts.add_params(params);
}

entrypoint_codegen::facts::LayoutRange add_layouts(EntrypointFacts& facts, std::span<const LayoutDecl> layouts) {
  return facts.add_layouts(layouts);
}

FunctionDecl make_function(EntrypointFacts& facts, SourceLocation location, BridgeKind bridge_kind) {
  return FunctionDecl{
      .name = facts.add_string("prompp_fn"),
      .return_type_spelling = facts.add_string("void"),
      .documentation = facts.add_string(""),
      .bridge_kind = bridge_kind,
      .params = add_params(facts, {}),
      .layouts = add_layouts(facts, {}),
      .location = location,
      .has_c_linkage = true,
  };
}

TEST(ValidateEntrypointsTest, AddsMissingEntrypointAttributeForUnannotatedFunction) {
  // Arrange
  EntrypointFacts facts;
  const SourceLocation location = add_source_file(facts);
  const auto function_id = facts.add_function(make_function(facts, location, BridgeKind::kUnknown));

  // Act
  entrypoint_codegen::validate::validate_entrypoints(facts);

  // Assert
  const auto diagnostics = facts.diagnostics();
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kMissingEntrypointAttribute);
  EXPECT_EQ(diagnostics[0].severity, Severity::kError);
  EXPECT_EQ(diagnostics[0].function, function_id);
  EXPECT_EQ(diagnostics[0].location.line, location.line);
}

TEST(ValidateEntrypointsTest, AddsMissingArgumentsLayoutWhenArgsParameterHasNoLayout) {
  // Arrange
  EntrypointFacts facts;
  const SourceLocation location = add_source_file(facts);
  const std::vector<ParamDecl> params{
      ParamDecl{
          .name = facts.add_string("args"),
          .type_spelling = facts.add_string("void*"),
          .role = ParamRole::kArgs,
          .location = location,
      },
  };
  FunctionDecl function = make_function(facts, location, BridgeKind::kFastCGo);
  function.params = add_params(facts, params);
  const auto function_id = facts.add_function(function);

  // Act
  entrypoint_codegen::validate::validate_entrypoints(facts);

  // Assert
  const auto diagnostics = facts.diagnostics();
  ASSERT_EQ(diagnostics.size(), 1);
  EXPECT_EQ(diagnostics[0].code, DiagnosticCode::kMissingArgumentsLayout);
  EXPECT_EQ(diagnostics[0].function, function_id);
  EXPECT_EQ(diagnostics[0].location.line, location.line);
}

}  // namespace
