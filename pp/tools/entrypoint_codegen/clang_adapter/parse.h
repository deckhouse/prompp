#pragma once

#include "diagnostics/diagnostics.h"
#include "facts/entrypoint_facts.h"

#include <filesystem>
#include <memory_resource>
#include <string>
#include <vector>

namespace entrypoint_codegen::clang_adapter {

struct ParseOptions {
  std::vector<std::filesystem::path> source_files;
  std::vector<std::string> clang_args;
  std::pmr::memory_resource* memory_resource = std::pmr::get_default_resource();
};

facts::EntrypointFacts parse_files(const ParseOptions& options, diagnostics::DiagnosticSet& diagnostic_set);

}  // namespace entrypoint_codegen::clang_adapter
