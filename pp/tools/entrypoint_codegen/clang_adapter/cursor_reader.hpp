#pragma once

#include "facts/entrypoint_facts.h"
#include "facts/facts.h"

#include <clang-c/Index.h>

#include <string_view>

namespace entrypoint_codegen::clang_adapter {

inline facts::StringId add_cursor_spelling(facts::EntrypointFacts& facts, CXCursor cursor) {
  CXString spelling = clang_getCursorSpelling(cursor);
  const char* raw = clang_getCString(spelling);
  const facts::StringId id = facts.add_string(raw == nullptr ? std::string_view() : std::string_view(raw));
  clang_disposeString(spelling);
  return id;
}

inline facts::StringId add_type_spelling(facts::EntrypointFacts& facts, CXType type) {
  CXString spelling = clang_getTypeSpelling(type);
  const char* raw = clang_getCString(spelling);
  const facts::StringId id = facts.add_string(raw == nullptr ? std::string_view() : std::string_view(raw));
  clang_disposeString(spelling);
  return id;
}

inline facts::StringId add_cursor_raw_comment(facts::EntrypointFacts& facts, CXCursor cursor) {
  CXString comment = clang_Cursor_getRawCommentText(cursor);
  const char* raw = clang_getCString(comment);
  const facts::StringId id = facts.add_string(raw == nullptr ? std::string_view() : std::string_view(raw));
  clang_disposeString(comment);
  return id;
}

inline bool cursor_spelling_equals(CXCursor cursor, std::string_view expected) {
  CXString spelling = clang_getCursorSpelling(cursor);
  const char* raw = clang_getCString(spelling);
  const bool result = (raw == nullptr ? std::string_view() : std::string_view(raw)) == expected;
  clang_disposeString(spelling);
  return result;
}

inline bool cursor_spelling_starts_with(CXCursor cursor, std::string_view prefix) {
  CXString spelling = clang_getCursorSpelling(cursor);
  const char* raw = clang_getCString(spelling);
  const std::string_view view = raw == nullptr ? std::string_view() : std::string_view(raw);
  const bool result = view.substr(0, prefix.size()) == prefix;
  clang_disposeString(spelling);
  return result;
}

inline bool has_c_linkage(CXCursor function_cursor) {
  return clang_getCursorLanguage(function_cursor) == CXLanguage_C;
}

}  // namespace entrypoint_codegen::clang_adapter
