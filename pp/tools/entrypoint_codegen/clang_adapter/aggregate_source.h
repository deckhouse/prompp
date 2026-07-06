#pragma once

#include <filesystem>
#include <memory_resource>
#include <span>
#include <string>

namespace entrypoint_codegen::clang_adapter {

struct AggregateSource {
  std::pmr::string path;
  std::pmr::string contents;
};

[[nodiscard]] AggregateSource build_aggregate_source(std::span<const std::filesystem::path> source_files, std::pmr::memory_resource* memory_resource);

}  // namespace entrypoint_codegen::clang_adapter
