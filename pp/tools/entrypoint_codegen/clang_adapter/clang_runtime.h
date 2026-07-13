#pragma once

#include "clang_adapter/parse_options.h"
#include "diagnostics/diagnostics.h"
#include "facts/fact_arena.h"
#include "facts/facts.h"

#include <cstdint>
#include <filesystem>
#include <functional>
#include <memory_resource>
#include <span>
#include <string>
#include <string_view>
#include <utility>
#include <vector>

namespace epgen::clang_adapter {

class AstContext;
class CursorView;

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

namespace detail {

using ChildVisitor = std::function<VisitResult(CursorView, CursorView)>;

void visit_children_impl(CursorView cursor, const ChildVisitor& visitor);

}  // namespace detail

[[nodiscard]] std::string normalize_path(std::string path);

class TypeView {
 public:
  TypeView() = default;

  [[nodiscard]] std::string spelling() const;
  [[nodiscard]] CursorView canonical_declaration() const;

 private:
  friend class AstContext;
  friend class CursorView;

  TypeView(AstContext* context, uint32_t index) : context_(context), index_(index) {}

  AstContext* context_ = nullptr;
  uint32_t index_ = 0;
};

class CursorView {
 public:
  CursorView() = default;

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
  friend class AstContext;
  friend class ParseSession;
  friend void detail::visit_children_impl(CursorView cursor, const detail::ChildVisitor& visitor);

  CursorView(AstContext* context, uint32_t index) : context_(context), index_(index) {}

  AstContext* context_ = nullptr;
  uint32_t index_ = 0;
};

template <class Payload, class Callable>
void visit_children(CursorView cursor, Payload& payload, Callable&& callable) {
  detail::ChildVisitor visitor = [&payload, fn = std::forward<Callable>(callable)](CursorView child, CursorView parent) mutable {
    return fn(payload, child, parent);
  };
  detail::visit_children_impl(cursor, visitor);
}

struct VirtualParseInput {
  std::span<const std::filesystem::path> source_files;
  std::string_view virtual_source_path;
  std::string_view virtual_source;
};

class ParseSession {
 public:
  ParseSession(const ParseOptions& options, diagnostics::DiagnosticSet& diagnostic_set, facts::FactArena& facts, VirtualParseInput input);
  ~ParseSession();

  ParseSession(const ParseSession&) = delete;
  ParseSession& operator=(const ParseSession&) = delete;

  [[nodiscard]] std::pmr::memory_resource* memory_resource() const { return memory_resource_; }
  facts::FactArena& facts() { return facts_; }
  [[nodiscard]] const facts::FactArena& facts() const { return facts_; }

  void add_clang_diagnostics();

  [[nodiscard]] CursorView root_cursor() const;
  [[nodiscard]] facts::SourceLocation source_location_for(CursorView cursor);
  [[nodiscard]] bool is_input_source_file(CursorView cursor);

 private:
  class Impl;

  std::pmr::memory_resource* memory_resource_;
  diagnostics::DiagnosticSet& diagnostics_;
  facts::FactArena& facts_;
  Impl* impl_ = nullptr;
};

}  // namespace epgen::clang_adapter
