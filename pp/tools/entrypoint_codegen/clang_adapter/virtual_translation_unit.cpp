#include "clang_adapter/virtual_translation_unit.h"

namespace epgen::clang_adapter {

namespace {

void append_escaped_include_path(std::string& out, std::string_view path) {
  for (const char ch : path) {
    if (ch == '\\' || ch == '"') {
      out += '\\';
    }
    out += ch;
  }
}

}  // namespace

VirtualTranslationUnit build_virtual_translation_unit(std::span<const std::filesystem::path> source_files) {
  const std::filesystem::path virtual_source_path = std::filesystem::temp_directory_path() / "entrypoint_codegen_aggregate.cpp";
  VirtualTranslationUnit source{
      .path = virtual_source_path.string(),
      .contents = {},
  };

  for (const std::filesystem::path& file : source_files) {
    source.contents += "#include \"";
    append_escaped_include_path(source.contents, file.string());
    source.contents += "\"\n";
  }
  return source;
}

}  // namespace epgen::clang_adapter
