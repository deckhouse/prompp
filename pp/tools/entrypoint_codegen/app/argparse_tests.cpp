#include "app/argparse.h"

#include <gtest/gtest.h>

#include <cstdlib>
#include <filesystem>
#include <fstream>
#include <initializer_list>
#include <string>
#include <string_view>
#include <vector>

namespace {

epgen::app::CliOptions parse_args(std::initializer_list<std::string_view> args) {
  std::vector<std::string> argv_storage;
  argv_storage.reserve(args.size());
  for (std::string_view arg : args) {
    argv_storage.emplace_back(arg);
  }

  std::vector<char*> argv;
  argv.reserve(argv_storage.size());
  for (std::string& arg : argv_storage) {
    argv.push_back(arg.data());
  }

  return epgen::app::parse_arguments(static_cast<int>(argv.size()), argv.data());
}

std::filesystem::path test_tmp_dir() {
  if (const char* test_tmpdir = std::getenv("TEST_TMPDIR"); test_tmpdir != nullptr) {
    return test_tmpdir;
  }
  return std::filesystem::temp_directory_path();
}

std::filesystem::path touch_file(std::string_view name) {
  const std::filesystem::path path = test_tmp_dir() / name;
  std::ofstream out(path, std::ios::trunc);
  return path;
}

TEST(ArgparseTest, ReturnsHelpForHelpFlag) {
  // Act
  const epgen::app::CliOptions options = parse_args({"entrypoint_codegen", "--help"});

  // Assert
  EXPECT_TRUE(options.help);
  EXPECT_TRUE(options.run_options.analysis.source_files.empty());
}

TEST(ArgparseTest, CollectsExistingCppInputFiles) {
  // Arrange
  const std::filesystem::path source_file = touch_file("entrypoint_argparse.cpp");
  touch_file("entrypoint_argparse.txt");

  // Act
  const epgen::app::CliOptions options = parse_args({"entrypoint_codegen", source_file.string()});
  const auto source_files = options.run_options.analysis.source_files;

  // Assert
  ASSERT_EQ(source_files.size(), 1);
  EXPECT_EQ(source_files[0], std::filesystem::absolute(source_file).lexically_normal());
}

TEST(ArgparseTest, CollectsCppInputFilesRecursivelyFromDirectory) {
  // Arrange
  const std::filesystem::path root = test_tmp_dir() / "entrypoint_argparse_recursive";
  const std::filesystem::path nested = root / "nested";
  std::filesystem::create_directories(nested);
  const std::filesystem::path source_file = nested / "entrypoint_argparse_nested.cpp";
  std::ofstream out(source_file, std::ios::trunc);

  // Act
  const epgen::app::CliOptions options = parse_args({"entrypoint_codegen", root.string()});
  const auto source_files = options.run_options.analysis.source_files;

  // Assert
  ASSERT_EQ(source_files.size(), 1);
  EXPECT_EQ(source_files[0], std::filesystem::absolute(source_file).lexically_normal());
}

TEST(ArgparseTest, RejectsMissingInputPath) {
  // Arrange
  const std::filesystem::path missing = test_tmp_dir() / "entrypoint_argparse_missing.cpp";
  std::filesystem::remove(missing);

  // Act / Assert
  EXPECT_THROW(parse_args({"entrypoint_codegen", missing.string()}), std::runtime_error);
}

TEST(ArgparseTest, ParsesOutputDirectoryAsFactsFilePath) {
  // Arrange
  const std::filesystem::path output_dir = test_tmp_dir() / "facts";

  // Act
  const epgen::app::CliOptions options = parse_args({"entrypoint_codegen", "--output-dir=" + output_dir.string()});

  // Assert
  EXPECT_EQ(options.run_options.output.output_path, output_dir / "entrypoint_facts.json");
}

TEST(ArgparseTest, ParsesCheckModeAlias) {
  // Act
  const epgen::app::CliOptions options = parse_args({"entrypoint_codegen", "--no-output"});

  // Assert
  EXPECT_EQ(options.run_options.output.output_mode, epgen::app::OutputMode::kCheck);
}

TEST(ArgparseTest, CollectsClangArgsFromFlagAndSeparator) {
  // Act
  const epgen::app::CliOptions options = parse_args({"entrypoint_codegen", "--clang-arg=-I.", "--", "-std=c++2b"});
  const auto clang_args = options.run_options.analysis.clang_args;

  // Assert
  ASSERT_EQ(clang_args.size(), 2);
  EXPECT_EQ(clang_args[0], "-I.");
  EXPECT_EQ(clang_args[1], "-std=c++2b");
}

TEST(ArgparseTest, RejectsUnknownOutputMode) {
  // Act / Assert
  EXPECT_THROW(parse_args({"entrypoint_codegen", "--mode=xml"}), std::runtime_error);
}

TEST(ArgparseTest, ParsesRuntimeDebugPolicy) {
  // Act
  const epgen::app::CliOptions options = parse_args({"entrypoint_codegen", "--runtime-debug"});

  // Assert
  EXPECT_TRUE(options.run_options.runtime.debug_diagnostics);
}

}  // namespace
