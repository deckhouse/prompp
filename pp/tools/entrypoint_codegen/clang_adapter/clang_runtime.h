#pragma once

#include "clang_adapter/parse.h"
#include "diagnostics/diagnostics.h"
#include "facts/entrypoint_facts.h"
#include "facts/facts.h"

#include <clang-c/Index.h>

#include <cstdint>
#include <filesystem>
#include <memory_resource>
#include <span>
#include <string>
#include <string_view>
#include <vector>

namespace entrypoint_codegen::clang_adapter {

enum class CursorKind : uint8_t {
  kOther,
  kFunctionDecl,
  kAnnotateAttr,
  kFieldDecl,
  kStructDecl,
  kTypeAliasDecl,
};

enum class VisitResult : uint8_t {
  kBreak,
  kContinue,
  kRecurse,
};

[[nodiscard]] std::string normalize_path(std::string path);

class CursorView;

class TypeView {
 public:
  explicit TypeView(CXType type) : type_(type) {}

  [[nodiscard]] std::string spelling() const;
  [[nodiscard]] CursorView canonical_declaration() const;

 private:
  friend class CursorView;

  CXType type_;
};

class CursorView {
 public:
  explicit CursorView(CXCursor cursor) : cursor_(cursor) {}

  [[nodiscard]] std::string spelling() const;
  [[nodiscard]] std::string raw_comment() const;
  [[nodiscard]] TypeView type() const;
  [[nodiscard]] TypeView result_type() const;
  [[nodiscard]] CursorKind kind() const;
  [[nodiscard]] bool is_null() const;
  [[nodiscard]] bool is_definition() const;
  [[nodiscard]] bool has_c_language() const;
  [[nodiscard]] int argument_count() const;
  [[nodiscard]] CursorView argument(int index) const;

 private:
  friend class ParseSession;
  friend void visit_children(CursorView cursor, VisitResult (*visitor)(CursorView cursor, CursorView parent, void* data), void* data);

  CXCursor cursor_;
};

void visit_children(CursorView cursor, VisitResult (*visitor)(CursorView cursor, CursorView parent, void* data), void* data);

class ParseSession {
 public:
  ParseSession(const ParseOptions& options,
               diagnostics::DiagnosticSet& diagnostic_set,
               facts::EntrypointFacts& facts,
               const std::filesystem::path& source_file);
  ParseSession(const ParseOptions& options,
               diagnostics::DiagnosticSet& diagnostic_set,
               facts::EntrypointFacts& facts,
               std::span<const std::filesystem::path> source_files,
               std::string_view virtual_source_path,
               std::string_view virtual_source);
  ~ParseSession();

  ParseSession(const ParseSession&) = delete;
  ParseSession& operator=(const ParseSession&) = delete;

  [[nodiscard]] std::pmr::memory_resource* memory_resource() const { return memory_resource_; }
  facts::EntrypointFacts& facts() { return facts_; }
  [[nodiscard]] const facts::EntrypointFacts& facts() const { return facts_; }

  void add_clang_diagnostics();

  [[nodiscard]] CursorView root_cursor() const;
  [[nodiscard]] facts::SourceLocation source_location_for(CursorView cursor);
  [[nodiscard]] bool is_input_source_file(CursorView cursor);

 private:
  class Impl;

  std::pmr::memory_resource* memory_resource_;
  diagnostics::DiagnosticSet& diagnostics_;
  facts::EntrypointFacts& facts_;
  Impl* impl_ = nullptr;
};

}  // namespace entrypoint_codegen::clang_adapter
