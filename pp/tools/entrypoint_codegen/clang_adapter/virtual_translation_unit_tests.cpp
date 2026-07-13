#include "clang_adapter/virtual_translation_unit.h"

#include <gtest/gtest.h>

#include <array>
#include <filesystem>
#include <memory_resource>

namespace {

TEST(VirtualTranslationUnitTest, BuildsIncludesForSourceFilesInInputOrder) {
  // Arrange
  std::pmr::monotonic_buffer_resource memory_resource;
  const std::array<std::filesystem::path, 2> source_files{
      std::filesystem::path("first.cpp"),
      std::filesystem::path("second.cpp"),
  };

  // Act
  const epgen::clang_adapter::VirtualTranslationUnit unit = epgen::clang_adapter::build_virtual_translation_unit(source_files, &memory_resource);

  // Assert
  EXPECT_EQ(unit.path, "/tmp/entrypoint_codegen_aggregate.cpp");
  EXPECT_EQ(unit.contents, "#include \"first.cpp\"\n#include \"second.cpp\"\n");
}

TEST(VirtualTranslationUnitTest, EscapesIncludePathCharacters) {
  // Arrange
  std::pmr::monotonic_buffer_resource memory_resource;
  const std::array<std::filesystem::path, 1> source_files{
      std::filesystem::path("dir/quote\"and\\slash.cpp"),
  };

  // Act
  const epgen::clang_adapter::VirtualTranslationUnit unit = epgen::clang_adapter::build_virtual_translation_unit(source_files, &memory_resource);

  // Assert
  EXPECT_EQ(unit.contents, "#include \"dir/quote\\\"and\\\\slash.cpp\"\n");
}

}  // namespace
