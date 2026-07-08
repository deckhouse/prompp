#include "clang_adapter/aggregate_source.h"

namespace epgen::clang_adapter {

AggregateSource build_aggregate_source(std::span<const std::filesystem::path> source_files, std::pmr::memory_resource* memory_resource) {
  AggregateSource source{
      .path = std::pmr::string("/tmp/entrypoint_codegen_aggregate.cpp", memory_resource),
      .contents = std::pmr::string(memory_resource),
  };

  for (const std::filesystem::path& file : source_files) {
    source.contents += "#include \"";
    source.contents += file.string();
    source.contents += "\"\n";
  }
  return source;
}

}  // namespace epgen::clang_adapter
