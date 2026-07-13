#pragma once

#include <filesystem>
#include <memory_resource>
#include <string>
#include <vector>

namespace epgen::clang_adapter {

struct ParseOptions {
  std::vector<std::filesystem::path> source_files;
  std::vector<std::string> clang_args;
  std::pmr::memory_resource* memory_resource = std::pmr::get_default_resource();
};

}  // namespace epgen::clang_adapter
