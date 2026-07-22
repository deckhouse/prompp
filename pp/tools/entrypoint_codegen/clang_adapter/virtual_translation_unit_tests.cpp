#include "clang_adapter/virtual_translation_unit.h"

#include <gtest/gtest.h>

#include <array>
#include <filesystem>
#include <memory_resource>
#include <string>
#include <string_view>

namespace {

TEST(VirtualTranslationUnitTest, BuildsIncludesForSourceFilesInInputOrder) {
  // Arrange
  std::pmr::monotonic_buffer_resource memory_resource;
  const std::array<std::filesystem::path, 2> source_files{
      std::filesystem::path("first.cpp"),
      std::filesystem::path("second.cpp"),
  };
  const std::string expected_path = (std::filesystem::temp_directory_path() / "entrypoint_codegen_aggregate.cpp").string();

  // Act
  const epgen::clang_adapter::VirtualTranslationUnit unit = epgen::clang_adapter::build_virtual_translation_unit(source_files, &memory_resource);

  // Assert
  EXPECT_EQ(std::string_view(unit.path.data(), unit.path.size()), std::string_view(expected_path));
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
