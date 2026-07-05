#pragma once

#include "clang_adapter/cursor_reader.hpp"
#include "clang_adapter/session.hpp"
#include "facts/facts.h"

#include <clang-c/Index.h>

#include <string_view>
#include <vector>

namespace entrypoint_codegen::clang_adapter {

struct ParamNameAndRole {
  facts::StringId name;
  facts::ParamRole role;
};

inline facts::ParamRole param_role_for(std::string_view name) {
  if (name == "args") {
    return facts::ParamRole::kArgs;
  }
  if (name == "res") {
    return facts::ParamRole::kRes;
  }
  return facts::ParamRole::kOther;
}

inline ParamNameAndRole add_param_name_and_role(facts::EntrypointFacts& facts, CXCursor cursor) {
  CXString spelling = clang_getCursorSpelling(cursor);
  const char* raw = clang_getCString(spelling);
  const std::string_view name = raw == nullptr ? std::string_view() : std::string_view(raw);
  const ParamNameAndRole result{
      .name = facts.add_string(name),
      .role = param_role_for(name),
  };
  clang_disposeString(spelling);
  return result;
}

inline facts::BridgeKind bridge_kind_from_annotation(std::string_view annotation) {
  if (annotation == "prompp.entrypoint.cgo") {
    return facts::BridgeKind::kCGo;
  }
  if (annotation == "prompp.entrypoint.fastcgo") {
    return facts::BridgeKind::kFastCGo;
  }
  return facts::BridgeKind::kUnknown;
}

inline facts::BridgeKind bridge_kind_from_annotation_cursor(CXCursor cursor) {
  CXString spelling = clang_getCursorSpelling(cursor);
  const char* raw = clang_getCString(spelling);
  const facts::BridgeKind bridge_kind = bridge_kind_from_annotation(raw == nullptr ? std::string_view() : std::string_view(raw));
  clang_disposeString(spelling);
  return bridge_kind;
}

class FunctionExtractor {
 public:
  explicit FunctionExtractor(ParseSession& session) : session_(session) {}

  [[nodiscard]] bool is_candidate_function(CXCursor function_cursor) const {
    return has_c_linkage(function_cursor) || cursor_spelling_starts_with(function_cursor, "prompp_");
  }

  void add_function(CXCursor function_cursor) {
    std::pmr::vector<facts::ParamDecl> params = extract_params(function_cursor);

    const facts::BridgeKind bridge_kind = extract_bridge_kind(function_cursor);
    std::pmr::vector<facts::LayoutDecl> layouts(session_.memory_resource());
    if (bridge_kind == facts::BridgeKind::kFastCGo) {
      layouts = extract_layouts(function_cursor);
    }

    const CXType function_type = clang_getCursorType(function_cursor);
    session_.facts().add_function(facts::FunctionDecl{
        .name = add_cursor_spelling(session_.facts(), function_cursor),
        .return_type_spelling = add_type_spelling(session_.facts(), clang_getResultType(function_type)),
        .documentation = add_cursor_raw_comment(session_.facts(), function_cursor),
        .bridge_kind = bridge_kind,
        .params = session_.facts().add_params(params),
        .layouts = session_.facts().add_layouts(layouts),
        .location = session_.source_location_for(clang_getCursorLocation(function_cursor)),
        .has_c_linkage = has_c_linkage(function_cursor),
    });
  }

 private:
  [[nodiscard]] facts::BridgeKind extract_bridge_kind(CXCursor function_cursor) {
    struct BridgeVisitorState {
      facts::BridgeKind bridge_kind = facts::BridgeKind::kUnknown;
    } visitor_state;

    clang_visitChildren(
        function_cursor,
        [](CXCursor cursor, CXCursor /*parent*/, CXClientData data) {
          if (clang_getCursorKind(cursor) != CXCursor_AnnotateAttr) {
            return CXChildVisit_Continue;
          }

          auto& state = *static_cast<BridgeVisitorState*>(data);
          const facts::BridgeKind next = bridge_kind_from_annotation_cursor(cursor);
          if (next != facts::BridgeKind::kUnknown) {
            state.bridge_kind = next;
            return CXChildVisit_Break;
          }
          return CXChildVisit_Continue;
        },
        &visitor_state);

    return visitor_state.bridge_kind;
  }

  [[nodiscard]] std::pmr::vector<facts::ParamDecl> extract_params(CXCursor function_cursor) {
    std::pmr::vector<facts::ParamDecl> params(session_.memory_resource());
    const int count = clang_Cursor_getNumArguments(function_cursor);
    if (count <= 0) {
      return params;
    }

    params.reserve(static_cast<size_t>(count));
    for (int i = 0; i < count; ++i) {
      const CXCursor arg = clang_Cursor_getArgument(function_cursor, i);
      const ParamNameAndRole name_and_role = add_param_name_and_role(session_.facts(), arg);
      params.push_back(facts::ParamDecl{
          .name = name_and_role.name,
          .type_spelling = add_type_spelling(session_.facts(), clang_getCursorType(arg)),
          .role = name_and_role.role,
          .location = session_.source_location_for(clang_getCursorLocation(arg)),
      });
    }
    return params;
  }

  [[nodiscard]] std::pmr::vector<facts::LayoutDecl> extract_layouts(CXCursor function_cursor) {
    std::pmr::vector<facts::LayoutDecl> layouts(session_.memory_resource());
    struct LayoutVisitorState {
      FunctionExtractor& extractor;
      std::pmr::vector<facts::LayoutDecl>& layouts;
    } visitor_state{
        .extractor = *this,
        .layouts = layouts,
    };

    clang_visitChildren(
        function_cursor,
        [](CXCursor cursor, CXCursor /*parent*/, CXClientData data) {
          auto& state = *static_cast<LayoutVisitorState*>(data);
          if (state.extractor.try_append_layout(cursor, state.layouts)) {
            return CXChildVisit_Continue;
          }
          return CXChildVisit_Recurse;
        },
        &visitor_state);
    return layouts;
  }

  [[nodiscard]] std::pmr::vector<facts::FieldDecl> extract_fields(CXCursor struct_cursor) {
    std::pmr::vector<facts::FieldDecl> fields(session_.memory_resource());
    struct FieldVisitorState {
      FunctionExtractor& extractor;
      std::pmr::vector<facts::FieldDecl>& fields;
    } visitor_state{
        .extractor = *this,
        .fields = fields,
    };

    clang_visitChildren(
        struct_cursor,
        [](CXCursor cursor, CXCursor /*parent*/, CXClientData data) {
          if (clang_getCursorKind(cursor) != CXCursor_FieldDecl) {
            return CXChildVisit_Continue;
          }

          auto& state = *static_cast<FieldVisitorState*>(data);
          state.fields.push_back(facts::FieldDecl{
              .name = add_cursor_spelling(state.extractor.session_.facts(), cursor),
              .type_spelling = add_type_spelling(state.extractor.session_.facts(), clang_getCursorType(cursor)),
              .location = state.extractor.session_.source_location_for(clang_getCursorLocation(cursor)),
          });
          return CXChildVisit_Continue;
        },
        &visitor_state);
    return fields;
  }

  [[nodiscard]] std::pmr::vector<facts::FieldDecl> extract_fields_from_alias(CXCursor alias_cursor) {
    std::pmr::vector<facts::FieldDecl> fields(session_.memory_resource());
    struct AliasVisitorState {
      FunctionExtractor& extractor;
      std::pmr::vector<facts::FieldDecl>& fields;
      bool found = false;
    } visitor_state{
        .extractor = *this,
        .fields = fields,
    };

    clang_visitChildren(
        alias_cursor,
        [](CXCursor cursor, CXCursor /*parent*/, CXClientData data) {
          auto& state = *static_cast<AliasVisitorState*>(data);
          if (clang_getCursorKind(cursor) == CXCursor_StructDecl) {
            state.fields = state.extractor.extract_fields(cursor);
            state.found = true;
            return CXChildVisit_Break;
          }
          return CXChildVisit_Recurse;
        },
        &visitor_state);

    if (visitor_state.found) {
      return fields;
    }

    const CXType canonical_type = clang_getCanonicalType(clang_getCursorType(alias_cursor));
    const CXCursor declaration = clang_getTypeDeclaration(canonical_type);
    if (!clang_Cursor_isNull(declaration) && clang_getCursorKind(declaration) == CXCursor_StructDecl) {
      return extract_fields(declaration);
    }
    return fields;
  }

  void append_struct_layout(CXCursor struct_cursor, facts::LayoutKind kind, std::pmr::vector<facts::LayoutDecl>& layouts) {
    std::pmr::vector<facts::FieldDecl> fields = extract_fields(struct_cursor);
    layouts.push_back(facts::LayoutDecl{
        .kind = kind,
        .fields = session_.facts().add_fields(fields),
        .location = session_.source_location_for(clang_getCursorLocation(struct_cursor)),
    });
  }

  void append_alias_layout(CXCursor alias_cursor, facts::LayoutKind kind, std::pmr::vector<facts::LayoutDecl>& layouts) {
    std::pmr::vector<facts::FieldDecl> fields = extract_fields_from_alias(alias_cursor);
    if (fields.empty()) {
      return;
    }

    layouts.push_back(facts::LayoutDecl{
        .kind = kind,
        .fields = session_.facts().add_fields(fields),
        .location = session_.source_location_for(clang_getCursorLocation(alias_cursor)),
    });
  }

  [[nodiscard]] bool try_append_layout(CXCursor cursor, std::pmr::vector<facts::LayoutDecl>& layouts) {
    const CXCursorKind kind = clang_getCursorKind(cursor);
    if (kind == CXCursor_StructDecl) {
      if (cursor_spelling_equals(cursor, "Arguments")) {
        append_struct_layout(cursor, facts::LayoutKind::kArguments, layouts);
        return true;
      }
      if (cursor_spelling_equals(cursor, "Result")) {
        append_struct_layout(cursor, facts::LayoutKind::kResult, layouts);
        return true;
      }
      return false;
    }

    if (kind == CXCursor_TypeAliasDecl) {
      if (cursor_spelling_equals(cursor, "Arguments")) {
        append_alias_layout(cursor, facts::LayoutKind::kArguments, layouts);
        return true;
      }
      if (cursor_spelling_equals(cursor, "Result")) {
        append_alias_layout(cursor, facts::LayoutKind::kResult, layouts);
        return true;
      }
    }
    return false;
  }

  ParseSession& session_;
};

}  // namespace entrypoint_codegen::clang_adapter
