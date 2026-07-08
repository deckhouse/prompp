#include "clang_adapter/parse.h"

#include <gtest/gtest.h>

#include "diagnostics/diagnostics.h"

#include <cstdlib>
#include <filesystem>
#include <fstream>
#include <span>
#include <stdexcept>
#include <string>
#include <string_view>

namespace {

std::filesystem::path test_tmp_dir() {
  if (const char* test_tmpdir = std::getenv("TEST_TMPDIR"); test_tmpdir != nullptr) {
    return test_tmpdir;
  }
  return std::filesystem::temp_directory_path();
}

std::filesystem::path write_source_file(std::string_view name, std::string_view source) {
  const std::filesystem::path path = test_tmp_dir() / name;
  std::ofstream out(path, std::ios::trunc);
  out << source;
  return path;
}

TEST(ClangAdapterParseTest, RejectsEmptyInputList) {
  const epgen::clang_adapter::ParseOptions options;
  epgen::diagnostics::DiagnosticSet diagnostics;

  EXPECT_THROW(epgen::clang_adapter::parse_files(options, diagnostics), std::invalid_argument);
}

TEST(ClangAdapterParseTest, ExtractsFastCgoFunctionFactsFromSourceFile) {
  // Arrange
  const std::filesystem::path source_file = write_source_file("entrypoint_codegen_parse_test.cpp", R"cpp(
    extern "C" __attribute__((annotate("prompp.entrypoint.fastcgo"))) void prompp_store(void* args, void* res) {
      struct Arguments {
        int series;
      };
      struct Result {
        double value;
      };
    }
  )cpp");
  epgen::diagnostics::DiagnosticSet diagnostics;

  // Act
  epgen::facts::FactArena facts = epgen::clang_adapter::parse_files(
      epgen::clang_adapter::ParseOptions{
          .source_files = {source_file},
          .clang_args = {"-std=c++2b"},
      },
      diagnostics);
  const auto functions = facts.functions();
  const epgen::facts::FunctionDecl* function = functions.empty() ? nullptr : &functions[0];
  const std::span<const epgen::facts::ParamDecl> params = function == nullptr ? std::span<const epgen::facts::ParamDecl>() : facts.params(function->params);
  const std::span<const epgen::facts::LayoutDecl> layouts =
      function == nullptr ? std::span<const epgen::facts::LayoutDecl>() : facts.layouts(function->layouts);
  const std::span<const epgen::facts::FieldDecl> argument_fields =
      layouts.empty() ? std::span<const epgen::facts::FieldDecl>() : facts.fields(layouts[0].fields);
  const std::span<const epgen::facts::FieldDecl> result_fields =
      layouts.size() < 2 ? std::span<const epgen::facts::FieldDecl>() : facts.fields(layouts[1].fields);

  // Assert
  ASSERT_EQ(functions.size(), 1);
  ASSERT_NE(function, nullptr);
  EXPECT_EQ(facts.string(function->name), "prompp_store");
  EXPECT_EQ(function->bridge_kind, epgen::facts::BridgeKind::kFastCGo);
  EXPECT_TRUE(function->has_c_linkage);
  ASSERT_EQ(params.size(), 2);
  EXPECT_EQ(facts.string(params[0].name), "args");
  EXPECT_EQ(params[0].role, epgen::facts::ParamRole::kArgs);
  EXPECT_EQ(facts.string(params[1].name), "res");
  EXPECT_EQ(params[1].role, epgen::facts::ParamRole::kRes);
  ASSERT_EQ(layouts.size(), 2);
  EXPECT_EQ(layouts[0].kind, epgen::facts::LayoutKind::kArguments);
  EXPECT_EQ(layouts[1].kind, epgen::facts::LayoutKind::kResult);
  ASSERT_EQ(argument_fields.size(), 1);
  EXPECT_EQ(facts.string(argument_fields[0].name), "series");
  EXPECT_EQ(facts.string(argument_fields[0].type_spelling), "int");
  ASSERT_EQ(result_fields.size(), 1);
  EXPECT_EQ(facts.string(result_fields[0].name), "value");
  EXPECT_EQ(facts.string(result_fields[0].type_spelling), "double");
}

TEST(ClangAdapterParseTest, RecordsClangDiagnosticsSeparately) {
  // Arrange
  const std::filesystem::path source_file = write_source_file("entrypoint_codegen_invalid_parse_test.cpp", "int broken = ;\n");
  epgen::diagnostics::DiagnosticSet diagnostics;

  // Act
  epgen::clang_adapter::parse_files(
      epgen::clang_adapter::ParseOptions{
          .source_files = {source_file},
          .clang_args = {"-std=c++2b"},
      },
      diagnostics);
  const auto diagnostic_values = diagnostics.diagnostics();

  // Assert
  ASSERT_EQ(diagnostic_values.size(), 1);
  EXPECT_EQ(diagnostic_values[0].code, epgen::diagnostics::DiagnosticCode::kClangDiagnostic);
  EXPECT_EQ(diagnostic_values[0].severity, epgen::diagnostics::Severity::kError);
}

TEST(ClangAdapterParseTest, ExtractsFunctionsFromMultipleInputFiles) {
  // Arrange
  const std::filesystem::path first_source_file = write_source_file("entrypoint_codegen_batch_first.cpp", R"cpp(
    extern "C" __attribute__((annotate("prompp.entrypoint.cgo"))) void prompp_first() {}
  )cpp");
  const std::filesystem::path second_source_file = write_source_file("entrypoint_codegen_batch_second.cpp", R"cpp(
    extern "C" __attribute__((annotate("prompp.entrypoint.cgo"))) void prompp_second() {}
  )cpp");
  epgen::diagnostics::DiagnosticSet diagnostics;

  // Act
  epgen::facts::FactArena facts = epgen::clang_adapter::parse_files(
      epgen::clang_adapter::ParseOptions{
          .source_files = {first_source_file, second_source_file},
          .clang_args = {"-std=c++2b"},
      },
      diagnostics);
  const auto functions = facts.functions();

  // Assert
  EXPECT_TRUE(diagnostics.empty());
  ASSERT_EQ(functions.size(), 2);
  EXPECT_EQ(facts.string(functions[0].name), "prompp_first");
  EXPECT_EQ(facts.string(functions[1].name), "prompp_second");
}

TEST(ClangAdapterParseTest, ReportsAggregateTranslationUnitInternalLinkageCollisions) {
  // Arrange
  const std::filesystem::path first_source_file = write_source_file("entrypoint_codegen_collision_first.cpp", R"cpp(
    static int helper() {
      return 1;
    }
    extern "C" __attribute__((annotate("prompp.entrypoint.cgo"))) void prompp_first() {
      (void)helper();
    }
  )cpp");
  const std::filesystem::path second_source_file = write_source_file("entrypoint_codegen_collision_second.cpp", R"cpp(
    static int helper() {
      return 2;
    }
    extern "C" __attribute__((annotate("prompp.entrypoint.cgo"))) void prompp_second() {
      (void)helper();
    }
  )cpp");
  epgen::diagnostics::DiagnosticSet diagnostics;

  // Act
  epgen::clang_adapter::parse_files(
      epgen::clang_adapter::ParseOptions{
          .source_files = {first_source_file, second_source_file},
          .clang_args = {"-std=c++2b"},
      },
      diagnostics);
  const auto diagnostic_values = diagnostics.diagnostics();

  // Assert
  ASSERT_FALSE(diagnostic_values.empty());
  EXPECT_EQ(diagnostic_values[0].code, epgen::diagnostics::DiagnosticCode::kClangDiagnostic);
  EXPECT_EQ(diagnostic_values[0].severity, epgen::diagnostics::Severity::kError);
}

TEST(ClangAdapterParseTest, IgnoresUnannotatedExternCFunctionWithoutEntrypointPrefix) {
  // Arrange
  const std::filesystem::path source_file = write_source_file("entrypoint_codegen_c_helper.cpp", R"cpp(
    extern "C" void helper_for_c_abi() {}
  )cpp");
  epgen::diagnostics::DiagnosticSet diagnostics;

  // Act
  epgen::facts::FactArena facts = epgen::clang_adapter::parse_files(
      epgen::clang_adapter::ParseOptions{
          .source_files = {source_file},
          .clang_args = {"-std=c++2b"},
      },
      diagnostics);

  // Assert
  EXPECT_TRUE(facts.functions().empty());
}

TEST(ClangAdapterParseTest, ExtractsAnnotatedFunctionWithoutEntrypointPrefix) {
  // Arrange
  const std::filesystem::path source_file = write_source_file("entrypoint_codegen_annotated_without_prefix.cpp", R"cpp(
    extern "C" __attribute__((annotate("prompp.entrypoint.cgo"))) void store() {}
  )cpp");
  epgen::diagnostics::DiagnosticSet diagnostics;

  // Act
  epgen::facts::FactArena facts = epgen::clang_adapter::parse_files(
      epgen::clang_adapter::ParseOptions{
          .source_files = {source_file},
          .clang_args = {"-std=c++2b"},
      },
      diagnostics);
  const auto functions = facts.functions();

  // Assert
  ASSERT_EQ(functions.size(), 1);
  EXPECT_EQ(facts.string(functions[0].name), "store");
  EXPECT_EQ(functions[0].bridge_kind, epgen::facts::BridgeKind::kCGo);
}

}  // namespace
