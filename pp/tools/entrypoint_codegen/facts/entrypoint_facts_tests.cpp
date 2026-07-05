#include "facts/entrypoint_facts.h"

#include <gtest/gtest.h>

#include <vector>

namespace {

using entrypoint_codegen::facts::BridgeKind;
using entrypoint_codegen::facts::EntrypointFacts;
using entrypoint_codegen::facts::FunctionDecl;
using entrypoint_codegen::facts::LayoutKind;
using entrypoint_codegen::facts::LayoutDecl;
using entrypoint_codegen::facts::ParamDecl;
using entrypoint_codegen::facts::ParamRole;
using entrypoint_codegen::facts::SourceLocation;

TEST(EntrypointFactsTest, ResolvesFunctionRangesToStoredRecords) {
  // Arrange
  EntrypointFacts facts;
  const SourceLocation location{
      .file = facts.add_source_file("entrypoint.cpp"),
      .line = 3,
      .column = 5,
  };
  const std::vector<ParamDecl> params{
      ParamDecl{
          .name = facts.add_string("args"),
          .type_spelling = facts.add_string("void*"),
          .role = ParamRole::kArgs,
          .location = location,
      },
  };
  const std::vector<LayoutDecl> layouts{
      LayoutDecl{
          .kind = LayoutKind::kArguments,
          .fields = facts.add_fields({}),
          .location = location,
      },
  };
  const auto param_range = facts.add_params(params);
  const auto layout_range = facts.add_layouts(layouts);
  const auto function_id = facts.add_function(FunctionDecl{
      .name = facts.add_string("prompp_fn"),
      .return_type_spelling = facts.add_string("void"),
      .documentation = facts.add_string(""),
      .bridge_kind = BridgeKind::kFastCGo,
      .params = param_range,
      .layouts = layout_range,
      .location = location,
      .has_c_linkage = true,
  });

  // Act
  const auto stored_params = facts.params(function_id);
  const auto stored_layouts = facts.layouts(function_id);

  // Assert
  ASSERT_EQ(stored_params.size(), 1);
  EXPECT_EQ(facts.string(stored_params[0].name), "args");
  ASSERT_EQ(stored_layouts.size(), 1);
  EXPECT_EQ(stored_layouts[0].kind, LayoutKind::kArguments);
}

}  // namespace
