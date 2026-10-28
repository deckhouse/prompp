#include "facts/fact_store.h"

#include <gtest/gtest.h>

#include <span>
#include <string_view>
#include <utility>
#include <vector>

namespace {

using epgen::facts::BridgeKind;
using epgen::facts::FactStore;
using epgen::facts::FieldDecl;
using epgen::facts::FunctionDecl;
using epgen::facts::LayoutDecl;
using epgen::facts::LayoutKind;
using epgen::facts::ParamDecl;
using epgen::facts::ParamRole;
using epgen::facts::SourceFileId;
using epgen::facts::SourceLocation;

class FactStoreTest : public testing::Test {
 protected:
  FactStore facts_;
};

TEST(FactsTest, DefaultIdsAndFactsAreInvalid) {
  // Arrange
  const SourceLocation location;
  const epgen::facts::SourceFileDecl source_file;
  const ParamDecl param;
  const FieldDecl field;
  const LayoutDecl layout;
  const FunctionDecl function;

  // Assert
  EXPECT_FALSE(location.is_valid());
  EXPECT_FALSE(source_file.is_valid());
  EXPECT_FALSE(param.is_valid());
  EXPECT_FALSE(field.is_valid());
  EXPECT_FALSE(layout.is_valid());
  EXPECT_FALSE(function.is_valid());
}

TEST_F(FactStoreTest, StoresSourceFilesAndDirectStrings) {
  // Parsed source: entrypoint.cpp
  // void prompp_fn();

  // Act
  const std::string name = "prompp_fn";
  const auto source_file_id = facts_.add_source_file("entrypoint.cpp");
  const auto source_files = facts_.source_files();

  // Assert
  EXPECT_TRUE(source_file_id.is_valid());
  EXPECT_TRUE(facts_.source_file(source_file_id).is_valid());
  EXPECT_EQ(name, "prompp_fn");
  ASSERT_EQ(source_files.size(), 1);
  EXPECT_EQ(facts_.source_file(source_file_id).path, "entrypoint.cpp");
}

TEST_F(FactStoreTest, CopiesFunctionOwnedLists) {
  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 3, .column = 5};
  FunctionDecl function{
      .name = "prompp_fn",
      .return_type_spelling = "void",
      .documentation = "",
      .bridge_kind = BridgeKind::kFastCGo,
      .params =
          {
              ParamDecl{.name = "args", .type_spelling = "void*", .role = ParamRole::kArgs, .location = location},
              ParamDecl{.name = "res", .type_spelling = "void*", .role = ParamRole::kRes, .location = location},
          },
      .layouts =
          {
              LayoutDecl{
                  .kind = LayoutKind::kArguments,
                  .fields = {FieldDecl{.name = "series", .type_spelling = "int", .location = location}},
                  .location = location,
              },
          },
      .location = location,
      .has_c_linkage = true,
  };

  // Act
  const auto function_id = facts_.add_function(function);
  function.params[0].name = "changed";
  function.params.clear();
  function.layouts[0].fields[0].name = "changed";
  function.layouts[0].fields.clear();
  function.layouts.clear();
  const auto& stored_function = facts_.function(function_id);

  // Assert
  ASSERT_EQ(2U, stored_function.params.size());
  EXPECT_EQ("args", stored_function.params[0].name);
  EXPECT_EQ("res", stored_function.params[1].name);
  EXPECT_EQ(ParamRole::kArgs, stored_function.params[0].role);
  EXPECT_EQ(ParamRole::kRes, stored_function.params[1].role);

  ASSERT_EQ(1U, stored_function.layouts.size());
  ASSERT_EQ(1U, stored_function.layouts[0].fields.size());
  EXPECT_EQ("series", stored_function.layouts[0].fields[0].name);
}

TEST_F(FactStoreTest, ResolvesFunctionOwnedLists) {
  // Parsed source: entrypoint.cpp
  // void prompp_fn(void* args);

  // Arrange
  const SourceLocation location{.file = facts_.add_source_file("entrypoint.cpp"), .line = 3, .column = 5};
  const std::vector<ParamDecl> params{
      ParamDecl{.name = "args", .type_spelling = "void*", .role = ParamRole::kArgs, .location = location},
  };
  const std::vector<LayoutDecl> layouts{
      LayoutDecl{.kind = LayoutKind::kArguments, .fields = {}, .location = location},
  };

  // Act
  const auto function_id = facts_.add_function(FunctionDecl{
      .name = "prompp_fn",
      .return_type_spelling = "void",
      .documentation = "",
      .bridge_kind = BridgeKind::kFastCGo,
      .params = params,
      .layouts = layouts,
      .location = location,
      .has_c_linkage = true,
  });
  const auto functions = facts_.functions();
  const auto& stored_function = facts_.function(function_id);
  const auto& stored_params = stored_function.params;
  const auto& stored_layouts = stored_function.layouts;

  // Assert
  ASSERT_EQ(functions.size(), 1);
  EXPECT_EQ(facts_.function(function_id).name, "prompp_fn");

  ASSERT_EQ(stored_params.size(), 1);
  EXPECT_EQ(stored_params[0].name, "args");

  ASSERT_EQ(stored_layouts.size(), 1);
  EXPECT_EQ(stored_layouts[0].kind, LayoutKind::kArguments);
}

TEST_F(FactStoreTest, ResolvesInvalidSourceFileToSafeFallbackValue) {
  // Arrange
  const SourceFileId source_file_id;

  // Act
  const epgen::facts::SourceFileDecl& source_file = facts_.source_file(source_file_id);

  // Assert
  EXPECT_EQ(source_file.path, epgen::facts::kInvalidValuePlaceholder);
}

TEST_F(FactStoreTest, MoveTransfersStoredFacts) {
  // Parsed source: entrypoint.cpp
  // void prompp_fn();

  // Arrange
  const auto source_file_id = facts_.add_source_file("entrypoint.cpp");

  // Act
  FactStore moved = std::move(facts_);

  // Assert
  EXPECT_EQ(moved.source_file(source_file_id).path, "entrypoint.cpp");
}

}  // namespace
