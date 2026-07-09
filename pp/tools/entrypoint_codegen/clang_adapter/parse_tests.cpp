#include "clang_adapter/parse.h"

#include <gtest/gtest.h>

#include "diagnostics/diagnostics.h"

#include <cstdlib>
#include <filesystem>
#include <span>
#include <stdexcept>
#include <string>
#include <string_view>

namespace {

constexpr std::string_view kRunfilesRepo = "entrypoint_codegen~";
constexpr std::string_view kRunfilesTestDataDir = "clang_adapter/testdata";
constexpr std::string_view kSourceTreeTestDataDir = "tools/entrypoint_codegen/clang_adapter/testdata";

std::filesystem::path testdata_path(std::string_view name) {
  const std::filesystem::path source_tree_path = std::filesystem::path(kSourceTreeTestDataDir) / name;
  const char* test_srcdir = std::getenv("TEST_SRCDIR");
  if (test_srcdir == nullptr) {
    return source_tree_path;
  }

  const std::filesystem::path apparent_repo_path = std::filesystem::path(test_srcdir) / kRunfilesRepo / kRunfilesTestDataDir / name;
  if (std::filesystem::exists(apparent_repo_path)) {
    return apparent_repo_path;
  }

  const std::filesystem::path canonical_repo_path = std::filesystem::path(test_srcdir) / "_main" / "external" / kRunfilesRepo / kRunfilesTestDataDir / name;
  if (std::filesystem::exists(canonical_repo_path)) {
    return canonical_repo_path;
  }

  return apparent_repo_path;
}

epgen::facts::FactArena parse_one_file(const std::filesystem::path& source_file, epgen::diagnostics::DiagnosticSet& diagnostics) {
  return epgen::clang_adapter::parse_files(
      epgen::clang_adapter::ParseOptions{
          .source_files = {source_file},
          .clang_args = {"-std=c++2b"},
      },
      diagnostics);
}

epgen::facts::FactArena parse_two_files(const std::filesystem::path& first_source_file,
                                        const std::filesystem::path& second_source_file,
                                        epgen::diagnostics::DiagnosticSet& diagnostics) {
  return epgen::clang_adapter::parse_files(
      epgen::clang_adapter::ParseOptions{
          .source_files = {first_source_file, second_source_file},
          .clang_args = {"-std=c++2b"},
      },
      diagnostics);
}

void parse_empty_input(epgen::diagnostics::DiagnosticSet& diagnostics) {
  const epgen::clang_adapter::ParseOptions options;
  static_cast<void>(epgen::clang_adapter::parse_files(options, diagnostics));
}

TEST(ClangAdapterParseTest, RejectsEmptyInputList) {
  // Arrange
  epgen::diagnostics::DiagnosticSet diagnostics;

  // Act
  const auto parse = [&diagnostics] { parse_empty_input(diagnostics); };

  // Assert
  EXPECT_THROW(parse(), std::invalid_argument);
}

TEST(ClangAdapterParseTest, ExtractsFastCgoFunctionFactsFromSourceFile) {
  // Arrange
  const std::filesystem::path source_file = testdata_path("fastcgo_entrypoint.cpp");
  epgen::diagnostics::DiagnosticSet diagnostics;

  // Act
  epgen::facts::FactArena facts = parse_one_file(source_file, diagnostics);
  const auto functions = facts.functions();

  // Assert
  ASSERT_EQ(functions.size(), 1);
  const epgen::facts::FunctionDecl& function = functions[0];
  const std::span<const epgen::facts::ParamDecl> params = facts.params(function.params);
  const std::span<const epgen::facts::LayoutDecl> layouts = facts.layouts(function.layouts);
  EXPECT_EQ(facts.string(function.name), "prompp_store");
  EXPECT_EQ(function.bridge_kind, epgen::facts::BridgeKind::kFastCGo);
  EXPECT_TRUE(function.has_c_linkage);
  ASSERT_EQ(params.size(), 2);
  EXPECT_EQ(facts.string(params[0].name), "args");
  EXPECT_EQ(params[0].role, epgen::facts::ParamRole::kArgs);
  EXPECT_EQ(facts.string(params[1].name), "res");
  EXPECT_EQ(params[1].role, epgen::facts::ParamRole::kRes);
  ASSERT_EQ(layouts.size(), 2);
  EXPECT_EQ(layouts[0].kind, epgen::facts::LayoutKind::kArguments);
  EXPECT_EQ(layouts[1].kind, epgen::facts::LayoutKind::kResult);
  const std::span<const epgen::facts::FieldDecl> argument_fields = facts.fields(layouts[0].fields);
  const std::span<const epgen::facts::FieldDecl> result_fields = facts.fields(layouts[1].fields);
  ASSERT_EQ(argument_fields.size(), 1);
  EXPECT_EQ(facts.string(argument_fields[0].name), "series");
  EXPECT_EQ(facts.string(argument_fields[0].type_spelling), "int");
  ASSERT_EQ(result_fields.size(), 1);
  EXPECT_EQ(facts.string(result_fields[0].name), "value");
  EXPECT_EQ(facts.string(result_fields[0].type_spelling), "double");
}

TEST(ClangAdapterParseTest, RecordsClangDiagnosticsSeparately) {
  // Arrange
  const std::filesystem::path source_file = testdata_path("invalid_source.cpp");
  epgen::diagnostics::DiagnosticSet diagnostics;

  // Act
  static_cast<void>(parse_one_file(source_file, diagnostics));
  const auto diagnostic_values = diagnostics.diagnostics();

  // Assert
  ASSERT_EQ(diagnostic_values.size(), 1);
  EXPECT_EQ(diagnostic_values[0].code, epgen::diagnostics::DiagnosticCode::kClangDiagnostic);
  EXPECT_EQ(diagnostic_values[0].severity, epgen::diagnostics::Severity::kError);
}

TEST(ClangAdapterParseTest, ExtractsFunctionsFromMultipleInputFiles) {
  // Arrange
  const std::filesystem::path first_source_file = testdata_path("batch_first.cpp");
  const std::filesystem::path second_source_file = testdata_path("batch_second.cpp");
  epgen::diagnostics::DiagnosticSet diagnostics;

  // Act
  epgen::facts::FactArena facts = parse_two_files(first_source_file, second_source_file, diagnostics);
  const auto functions = facts.functions();

  // Assert
  EXPECT_TRUE(diagnostics.empty());
  ASSERT_EQ(functions.size(), 2);
  EXPECT_EQ(facts.string(functions[0].name), "prompp_first");
  EXPECT_EQ(facts.string(functions[1].name), "prompp_second");
}

TEST(ClangAdapterParseTest, ReportsAggregateTranslationUnitInternalLinkageCollisions) {
  // Arrange
  const std::filesystem::path first_source_file = testdata_path("collision_first.cpp");
  const std::filesystem::path second_source_file = testdata_path("collision_second.cpp");
  epgen::diagnostics::DiagnosticSet diagnostics;

  // Act
  static_cast<void>(parse_two_files(first_source_file, second_source_file, diagnostics));
  const auto diagnostic_values = diagnostics.diagnostics();

  // Assert
  ASSERT_FALSE(diagnostic_values.empty());
  EXPECT_EQ(diagnostic_values[0].code, epgen::diagnostics::DiagnosticCode::kClangDiagnostic);
  EXPECT_EQ(diagnostic_values[0].severity, epgen::diagnostics::Severity::kError);
}

TEST(ClangAdapterParseTest, IgnoresUnannotatedExternCFunctionWithoutEntrypointPrefix) {
  // Arrange
  const std::filesystem::path source_file = testdata_path("c_helper.cpp");
  epgen::diagnostics::DiagnosticSet diagnostics;

  // Act
  epgen::facts::FactArena facts = parse_one_file(source_file, diagnostics);

  // Assert
  EXPECT_TRUE(facts.functions().empty());
}

TEST(ClangAdapterParseTest, ExtractsAnnotatedFunctionWithoutEntrypointPrefix) {
  // Arrange
  const std::filesystem::path source_file = testdata_path("annotated_without_prefix.cpp");
  epgen::diagnostics::DiagnosticSet diagnostics;

  // Act
  epgen::facts::FactArena facts = parse_one_file(source_file, diagnostics);
  const auto functions = facts.functions();

  // Assert
  ASSERT_EQ(functions.size(), 1);
  EXPECT_EQ(facts.string(functions[0].name), "store");
  EXPECT_EQ(functions[0].bridge_kind, epgen::facts::BridgeKind::kCGo);
}

}  // namespace
