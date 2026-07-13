#include "clang_adapter/parse.h"

#include "clang_adapter/clang_runtime.h"
#include "clang_adapter/virtual_translation_unit.h"
#include "contract/entrypoint_contract.h"

#include <optional>
#include <stdexcept>
#include <string>
#include <string_view>
#include <vector>

namespace epgen::clang_adapter {

namespace {

struct ParamNameAndRole {
  facts::StringId name;
  facts::ParamRole role;
};

ParamNameAndRole add_param_name_and_role(facts::FactArena& facts, CursorView cursor) {
  const std::string name = cursor.spelling();
  return ParamNameAndRole{
      .name = facts.add_string(name),
      .role = contract::param_role_for_name(name),
  };
}

facts::BridgeKind bridge_kind_from_annotation_cursor(CursorView cursor) {
  return contract::bridge_kind_for_annotation(cursor.spelling());
}

class FunctionExtractor {
 public:
  explicit FunctionExtractor(ParseSession& session) : session_(session) {}

  [[nodiscard]] bool is_candidate_function(CursorView function_cursor) const {
    return contract::is_entrypoint_function_name(function_cursor.spelling()) || has_known_entrypoint_annotation(function_cursor);
  }

  void add_function(CursorView function_cursor) {
    std::pmr::vector<facts::ParamDecl> params = extract_params(function_cursor);

    const facts::BridgeKind bridge_kind = extract_bridge_kind(function_cursor);
    std::pmr::vector<facts::LayoutDecl> layouts(session_.memory_resource());
    if (bridge_kind == facts::BridgeKind::kFastCGo) {
      layouts = extract_layouts(function_cursor);
    }

    session_.facts().add_function(facts::FunctionDecl{
        .name = session_.facts().add_string(function_cursor.spelling()),
        .return_type_spelling = session_.facts().add_string(function_cursor.result_type().spelling()),
        .documentation = session_.facts().add_string(function_cursor.raw_comment()),
        .bridge_kind = bridge_kind,
        .params = session_.facts().add_params(params),
        .layouts = session_.facts().add_layouts(layouts),
        .location = session_.source_location_for(function_cursor),
        .has_c_linkage = function_cursor.has_c_language(),
    });
  }

 private:
  [[nodiscard]] facts::BridgeKind extract_bridge_kind(CursorView function_cursor) {
    struct BridgeVisitorState {
      facts::BridgeKind bridge_kind = facts::BridgeKind::kUnknown;
    } visitor_state;

    visit_children(function_cursor, visitor_state, [](BridgeVisitorState& state, CursorView cursor, CursorView /*parent*/) {
      if (cursor.kind() != CursorKind::kAnnotateAttr) {
        return VisitResult::kContinue;
      }

      const facts::BridgeKind next = bridge_kind_from_annotation_cursor(cursor);
      if (next != facts::BridgeKind::kUnknown) {
        state.bridge_kind = next;
        return VisitResult::kBreak;
      }
      return VisitResult::kContinue;
    });

    return visitor_state.bridge_kind;
  }

  [[nodiscard]] bool has_known_entrypoint_annotation(CursorView function_cursor) const {
    struct AnnotationVisitorState {
      bool found = false;
    } visitor_state;

    visit_children(function_cursor, visitor_state, [](AnnotationVisitorState& state, CursorView cursor, CursorView /*parent*/) {
      if (cursor.kind() != CursorKind::kAnnotateAttr) {
        return VisitResult::kContinue;
      }

      if (bridge_kind_from_annotation_cursor(cursor) != facts::BridgeKind::kUnknown) {
        state.found = true;
        return VisitResult::kBreak;
      }
      return VisitResult::kContinue;
    });

    return visitor_state.found;
  }

  [[nodiscard]] std::pmr::vector<facts::ParamDecl> extract_params(CursorView function_cursor) {
    std::pmr::vector<facts::ParamDecl> params(session_.memory_resource());
    const int count = function_cursor.argument_count();
    if (count <= 0) {
      return params;
    }

    params.reserve(static_cast<size_t>(count));
    for (int i = 0; i < count; ++i) {
      const CursorView arg = function_cursor.argument(i);
      const ParamNameAndRole name_and_role = add_param_name_and_role(session_.facts(), arg);
      params.push_back(facts::ParamDecl{
          .name = name_and_role.name,
          .type_spelling = session_.facts().add_string(arg.type().spelling()),
          .role = name_and_role.role,
          .location = session_.source_location_for(arg),
      });
    }
    return params;
  }

  [[nodiscard]] std::pmr::vector<facts::LayoutDecl> extract_layouts(CursorView function_cursor) {
    std::pmr::vector<facts::LayoutDecl> layouts(session_.memory_resource());
    struct LayoutVisitorState {
      FunctionExtractor& extractor;
      std::pmr::vector<facts::LayoutDecl>& layouts;
    } visitor_state{
        .extractor = *this,
        .layouts = layouts,
    };

    visit_children(function_cursor, visitor_state, [](LayoutVisitorState& state, CursorView cursor, CursorView /*parent*/) {
      if (state.extractor.try_append_layout(cursor, state.layouts)) {
        return VisitResult::kContinue;
      }
      return VisitResult::kRecurse;
    });
    return layouts;
  }

  [[nodiscard]] std::pmr::vector<facts::FieldDecl> extract_fields(CursorView struct_cursor) {
    std::pmr::vector<facts::FieldDecl> fields(session_.memory_resource());
    struct FieldVisitorState {
      FunctionExtractor& extractor;
      std::pmr::vector<facts::FieldDecl>& fields;
    } visitor_state{
        .extractor = *this,
        .fields = fields,
    };

    visit_children(struct_cursor, visitor_state, [](FieldVisitorState& state, CursorView field, CursorView /*parent*/) {
      if (field.kind() != CursorKind::kFieldDecl) {
        return VisitResult::kContinue;
      }

      state.fields.push_back(facts::FieldDecl{
          .name = state.extractor.session_.facts().add_string(field.spelling()),
          .type_spelling = state.extractor.session_.facts().add_string(field.type().spelling()),
          .location = state.extractor.session_.source_location_for(field),
      });
      return VisitResult::kContinue;
    });
    return fields;
  }

  [[nodiscard]] std::pmr::vector<facts::FieldDecl> extract_fields_from_alias(CursorView alias_cursor) {
    std::pmr::vector<facts::FieldDecl> fields(session_.memory_resource());
    struct AliasVisitorState {
      FunctionExtractor& extractor;
      std::pmr::vector<facts::FieldDecl>& fields;
      bool found = false;
    } visitor_state{
        .extractor = *this,
        .fields = fields,
    };

    visit_children(alias_cursor, visitor_state, [](AliasVisitorState& state, CursorView child, CursorView /*parent*/) {
      if (child.kind() == CursorKind::kStructDecl) {
        state.fields = state.extractor.extract_fields(child);
        state.found = true;
        return VisitResult::kBreak;
      }
      return VisitResult::kRecurse;
    });

    if (visitor_state.found) {
      return fields;
    }

    const CursorView declaration = alias_cursor.type().canonical_declaration();
    if (!declaration.is_null() && declaration.kind() == CursorKind::kStructDecl) {
      return extract_fields(declaration);
    }
    return fields;
  }

  void append_struct_layout(CursorView struct_cursor, facts::LayoutKind kind, std::pmr::vector<facts::LayoutDecl>& layouts) {
    std::pmr::vector<facts::FieldDecl> fields = extract_fields(struct_cursor);
    layouts.push_back(facts::LayoutDecl{
        .kind = kind,
        .fields = session_.facts().add_fields(fields),
        .location = session_.source_location_for(struct_cursor),
    });
  }

  void append_alias_layout(CursorView alias_cursor, facts::LayoutKind kind, std::pmr::vector<facts::LayoutDecl>& layouts) {
    std::pmr::vector<facts::FieldDecl> fields = extract_fields_from_alias(alias_cursor);
    layouts.push_back(facts::LayoutDecl{
        .kind = kind,
        .fields = session_.facts().add_fields(fields),
        .location = session_.source_location_for(alias_cursor),
    });
  }

  [[nodiscard]] bool try_append_layout(CursorView cursor, std::pmr::vector<facts::LayoutDecl>& layouts) {
    const CursorKind kind = cursor.kind();
    if (kind == CursorKind::kStructDecl) {
      const std::optional<facts::LayoutKind> layout_kind = contract::layout_kind_for_name(cursor.spelling());
      if (layout_kind.has_value()) {
        append_struct_layout(cursor, *layout_kind, layouts);
        return true;
      }
      return false;
    }

    if (kind == CursorKind::kTypeAliasDecl) {
      const std::optional<facts::LayoutKind> layout_kind = contract::layout_kind_for_name(cursor.spelling());
      if (layout_kind.has_value()) {
        append_alias_layout(cursor, *layout_kind, layouts);
        return true;
      }
    }
    return false;
  }

  ParseSession& session_;
};

void scan_translation_unit(ParseSession& session) {
  FunctionExtractor extractor(session);

  struct TranslationUnitScanState {
    ParseSession& session;
    FunctionExtractor& extractor;
  };

  TranslationUnitScanState scan_state{
      .session = session,
      .extractor = extractor,
  };

  visit_children(session.root_cursor(), scan_state, [](TranslationUnitScanState& state, CursorView function, CursorView /*parent*/) {
    if (function.kind() != CursorKind::kFunctionDecl || !function.is_definition()) {
      return VisitResult::kRecurse;
    }
    if (!state.session.is_input_source_file(function) || !state.extractor.is_candidate_function(function)) {
      return VisitResult::kContinue;
    }
    state.extractor.add_function(function);
    return VisitResult::kContinue;
  });
}

}  // namespace

facts::FactArena parse_files(const ParseOptions& options, diagnostics::DiagnosticSet& diagnostic_set) {
  if (options.source_files.empty()) {
    throw std::invalid_argument("no source files provided");
  }

  facts::FactArena facts(options.memory_resource);
  const VirtualTranslationUnit aggregate_source = build_virtual_translation_unit(options.source_files, options.memory_resource);
  ParseSession session(options, diagnostic_set, facts,
                       VirtualParseInput{
                           .source_files = options.source_files,
                           .virtual_source_path = aggregate_source.path,
                           .virtual_source = aggregate_source.contents,
                       });
  session.add_clang_diagnostics();
  scan_translation_unit(session);

  return facts;
}

}  // namespace epgen::clang_adapter
