#include "facts/entrypoint_facts.h"

#include <gtest/gtest.h>

#include <utility>
#include <vector>

namespace {

using entrypoint_codegen::facts::BridgeKind;
using entrypoint_codegen::facts::EntrypointFacts;
using entrypoint_codegen::facts::FieldDecl;
using entrypoint_codegen::facts::FunctionDecl;
using entrypoint_codegen::facts::LayoutDecl;
using entrypoint_codegen::facts::LayoutKind;
using entrypoint_codegen::facts::ParamDecl;
using entrypoint_codegen::facts::ParamRole;
using entrypoint_codegen::facts::SourceLocation;

class EntrypointFactsTest : public testing::Test {
 protected:
  EntrypointFacts facts_;
};

TEST_F(EntrypointFactsTest, StoresStringsAndSourceFiles) {
  // Act
  const auto string_id = facts_.add_string("prompp_fn");
  const auto source_file_id = facts_.add_source_file("entrypoint.cpp");
  const auto source_files = facts_.source_files();

  // Assert
  EXPECT_EQ(facts_.string(string_id), "prompp_fn");
  ASSERT_EQ(source_files.size(), 1);
  EXPECT_EQ(facts_.string(facts_.source_file(source_file_id).path), "entrypoint.cpp");
}

TEST_F(EntrypointFactsTest, ResolvesContiguousRangesToStoredRecords) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 3, .column = 5};
  const std::vector<ParamDecl> params{
      ParamDecl{.name = facts_.add_string("args"), .type_spelling = facts_.add_string("void*"), .role = ParamRole::kArgs, .location = location},
      ParamDecl{.name = facts_.add_string("res"), .type_spelling = facts_.add_string("void*"), .role = ParamRole::kRes, .location = location},
  };
  const std::vector<FieldDecl> fields{
      FieldDecl{.name = facts_.add_string("series"), .type_spelling = facts_.add_string("int"), .location = location},
  };

  // Act
  const auto param_range = facts_.add_params(params);
  const auto field_range = facts_.add_fields(fields);
  const auto stored_params = facts_.params(param_range);
  const auto stored_fields = facts_.fields(field_range);

  // Assert
  ASSERT_EQ(stored_params.size(), 2);
  EXPECT_EQ(facts_.string(stored_params[0].name), "args");
  EXPECT_EQ(stored_params[1].role, ParamRole::kRes);
  ASSERT_EQ(stored_fields.size(), 1);
  EXPECT_EQ(facts_.string(stored_fields[0].name), "series");
}

TEST_F(EntrypointFactsTest, ResolvesFunctionOwnedRanges) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 3, .column = 5};
  const std::vector<ParamDecl> params{
      ParamDecl{.name = facts_.add_string("args"), .type_spelling = facts_.add_string("void*"), .role = ParamRole::kArgs, .location = location},
  };
  const std::vector<LayoutDecl> layouts{
      LayoutDecl{.kind = LayoutKind::kArguments, .fields = facts_.add_fields({}), .location = location},
  };
  const auto param_range = facts_.add_params(params);
  const auto layout_range = facts_.add_layouts(layouts);

  // Act
  const auto function_id = facts_.add_function(FunctionDecl{
      .name = facts_.add_string("prompp_fn"),
      .return_type_spelling = facts_.add_string("void"),
      .documentation = facts_.add_string(""),
      .bridge_kind = BridgeKind::kFastCGo,
      .params = param_range,
      .layouts = layout_range,
      .location = location,
      .has_c_linkage = true,
  });
  const auto functions = facts_.functions();
  const auto stored_params = facts_.params(function_id);
  const auto stored_layouts = facts_.layouts(function_id);

  // Assert
  ASSERT_EQ(functions.size(), 1);
  EXPECT_EQ(facts_.string(facts_.function(function_id).name), "prompp_fn");
  ASSERT_EQ(stored_params.size(), 1);
  EXPECT_EQ(facts_.string(stored_params[0].name), "args");
  ASSERT_EQ(stored_layouts.size(), 1);
  EXPECT_EQ(stored_layouts[0].kind, LayoutKind::kArguments);
}

TEST_F(EntrypointFactsTest, MoveTransfersStoredFacts) {
  // Arrange
  const auto string_id = facts_.add_string("prompp_fn");

  // Act
  EntrypointFacts moved = std::move(facts_);

  // Assert
  EXPECT_EQ(moved.string(string_id), "prompp_fn");
}

}  // namespace
