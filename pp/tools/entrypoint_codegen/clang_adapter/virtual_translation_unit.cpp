#include "clang_adapter/virtual_translation_unit.h"

namespace epgen::clang_adapter {

namespace {

void append_escaped_include_path(std::pmr::string& out, std::string_view path) {
  for (const char ch : path) {
    if (ch == '\\' || ch == '"') {
      out += '\\';
    }
    out += ch;
  }
}

}  // namespace

VirtualTranslationUnit build_virtual_translation_unit(std::span<const std::filesystem::path> source_files, std::pmr::memory_resource* memory_resource) {
  VirtualTranslationUnit source{
      .path = std::pmr::string("/tmp/entrypoint_codegen_aggregate.cpp", memory_resource),
      .contents = std::pmr::string(memory_resource),
  };

  for (const std::filesystem::path& file : source_files) {
    source.contents += "#include \"";
    append_escaped_include_path(source.contents, file.string());
    source.contents += "\"\n";
  }
  return source;
}

}  // namespace epgen::clang_adapter
