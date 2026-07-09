#pragma once

#include <filesystem>
#include <memory_resource>
#include <span>
#include <string>

namespace epgen::clang_adapter {

struct VirtualTranslationUnit {
  std::pmr::string path;
  std::pmr::string contents;
};

[[nodiscard]] VirtualTranslationUnit build_virtual_translation_unit(std::span<const std::filesystem::path> source_files,
                                                                    std::pmr::memory_resource* memory_resource);

}  // namespace epgen::clang_adapter
