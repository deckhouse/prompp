#pragma once

#include <filesystem>
#include <string>
#include <vector>

namespace epgen::clang_adapter {

struct ParseOptions {
  std::vector<std::filesystem::path> source_files;
  std::vector<std::string> clang_args;
};

}  // namespace epgen::clang_adapter
