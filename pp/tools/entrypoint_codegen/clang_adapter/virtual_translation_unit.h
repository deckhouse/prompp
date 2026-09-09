#pragma once

#include <filesystem>
#include <span>
#include <string>

namespace epgen::clang_adapter {

struct VirtualTranslationUnit {
  std::string path;
  std::string contents;
};

[[nodiscard]] VirtualTranslationUnit build_virtual_translation_unit(std::span<const std::filesystem::path> source_files);

}  // namespace epgen::clang_adapter
