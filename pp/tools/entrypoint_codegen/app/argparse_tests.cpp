#include "app/argparse.h"

#include <gtest/gtest.h>

#include <filesystem>
#include <initializer_list>
#include <string>
#include <string_view>
#include <vector>

namespace {

class ArgparseTest : public testing::Test {
 protected:
  static epgen::app::CliOptions parse_args(std::initializer_list<std::string_view> args) {
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
};

TEST_F(ArgparseTest, ReturnsHelpForHelpFlag) {
  // Arrange

  // Act
  const epgen::app::CliOptions options = parse_args({"entrypoint_codegen", "--help"});

  // Assert
  EXPECT_TRUE(options.help);
  EXPECT_TRUE(options.run_options.analysis.source_files.empty());
}

TEST_F(ArgparseTest, ParsesJsonOutputPath) {
  // Arrange
  const std::filesystem::path output_path = "facts.json";

  // Act
  const epgen::app::CliOptions options = parse_args({"entrypoint_codegen", "--output=" + output_path.string()});

  // Assert
  EXPECT_EQ(options.run_options.output.output_path, output_path);
}

TEST_F(ArgparseTest, ParsesLintModeWithoutOutputPath) {
  // Arrange

  // Act
  const epgen::app::CliOptions options = parse_args({"entrypoint_codegen", "--mode=lint"});

  // Assert
  EXPECT_EQ(options.run_options.output.output_mode, epgen::app::OutputMode::kLint);
  EXPECT_TRUE(options.run_options.output.output_path.empty());
}

TEST_F(ArgparseTest, CollectsClangArgsAfterSeparator) {
  // Arrange

  // Act
  const epgen::app::CliOptions options = parse_args({"entrypoint_codegen", "--mode=lint", "--", "-std=c++2b"});
  const auto clang_args = options.run_options.analysis.clang_args;

  // Assert
  ASSERT_EQ(clang_args.size(), 1);
  EXPECT_EQ(clang_args[0], "-std=c++2b");
}

TEST_F(ArgparseTest, RejectsUnknownOutputMode) {
  // Arrange

  // Act
  const auto parse = [] { return parse_args({"entrypoint_codegen", "--mode=xml"}); };

  // Assert
  EXPECT_THROW(parse(), std::runtime_error);
}

TEST_F(ArgparseTest, ParsesRuntimeDebugPolicy) {
  // Arrange

  // Act
  const epgen::app::CliOptions options = parse_args({"entrypoint_codegen", "--mode=lint", "--runtime-debug"});

  // Assert
  EXPECT_TRUE(options.run_options.runtime.debug_diagnostics);
}

}  // namespace
