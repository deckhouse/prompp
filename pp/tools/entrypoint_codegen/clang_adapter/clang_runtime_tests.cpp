#include "clang_adapter/clang_runtime.h"

#include <gtest/gtest.h>

#include <filesystem>

namespace {

TEST(ClangRuntimeTest, NormalizesExecrootPathToWorkspacePath) {
  // Arrange
  const std::string path = "/tmp/bazel/execroot/_main/tools/entrypoint_codegen/input.cpp";

  // Act
  const std::string normalized = epgen::clang_adapter::normalize_path(path);

  // Assert
  EXPECT_EQ(normalized, "tools/entrypoint_codegen/input.cpp");
}

TEST(ClangRuntimeTest, KeepsExternalAbsolutePathUnchanged) {
  // Arrange
  const std::filesystem::path path = "/outside/workspace/input.cpp";

  // Act
  const std::string normalized = epgen::clang_adapter::normalize_path(path.string());

  // Assert
  EXPECT_EQ(normalized, path.string());
}

TEST(ClangRuntimeTest, KeepsRelativePathUnchanged) {
  // Arrange
  const std::string path = "tools/entrypoint_codegen/input.cpp";

  // Act
  const std::string normalized = epgen::clang_adapter::normalize_path(path);

  // Assert
  EXPECT_EQ(normalized, path);
}

}  // namespace
