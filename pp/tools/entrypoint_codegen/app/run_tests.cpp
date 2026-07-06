#include "app/run.h"

#include <gtest/gtest.h>

#include <cstdlib>
#include <filesystem>
#include <fstream>
#include <string_view>
#include <utility>

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

entrypoint_codegen::app::RunOptions check_options_for(std::filesystem::path source_file) {
  return entrypoint_codegen::app::RunOptions{
      .analysis =
          entrypoint_codegen::app::AnalysisOptions{
              .source_files = {std::move(source_file)},
              .clang_args = {"-std=c++2b"},
          },
      .output =
          entrypoint_codegen::app::OutputOptions{
              .output_path = {},
              .output_mode = entrypoint_codegen::app::OutputMode::kCheck,
          },
      .runtime = entrypoint_codegen::app::RuntimeOptions{},
  };
}

TEST(RunTest, CheckModeSucceedsForValidAnnotatedEntrypoint) {
  // Arrange
  const std::filesystem::path source_file = write_source_file("entrypoint_codegen_run_valid.cpp", R"cpp(
    extern "C" __attribute__((annotate("prompp.entrypoint.cgo"))) void prompp_store() {}
  )cpp");
  const entrypoint_codegen::app::RunOptions options = check_options_for(source_file);

  // Act
  const entrypoint_codegen::app::RunReport report = entrypoint_codegen::app::run(options);

  // Assert
  EXPECT_EQ(report.decision, entrypoint_codegen::app::ExitDecision::kSuccess);
  EXPECT_EQ(report.diagnostics.errors, 0);
  EXPECT_EQ(report.diagnostics.total, 0);
}

TEST(RunTest, CheckModeFailsWhenValidationReportsErrors) {
  // Arrange
  const std::filesystem::path source_file = write_source_file("entrypoint_codegen_run_invalid.cpp", R"cpp(
    extern "C" void prompp_store() {}
  )cpp");
  const entrypoint_codegen::app::RunOptions options = check_options_for(source_file);

  // Act
  const entrypoint_codegen::app::RunReport report = entrypoint_codegen::app::run(options);

  // Assert
  EXPECT_EQ(report.decision, entrypoint_codegen::app::ExitDecision::kAnalysisFailed);
  EXPECT_EQ(report.diagnostics.errors, 1);
  EXPECT_EQ(report.diagnostics.total, 1);
}

}  // namespace
