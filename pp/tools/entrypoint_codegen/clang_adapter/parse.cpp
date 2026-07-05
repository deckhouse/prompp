#include "clang_adapter/parse.h"

#include "clang_adapter/function_extractor.hpp"
#include "clang_adapter/session.hpp"

#include <clang-c/Index.h>

#include <stdexcept>
namespace entrypoint_codegen::clang_adapter {

facts::EntrypointFacts parse_files(const ParseOptions& options) {
  if (options.source_files.empty()) {
    throw std::invalid_argument("no source files provided");
  }

  ParseSession session(options);
  session.add_clang_diagnostics();

  FunctionExtractor extractor(session);

  struct TranslationUnitScanState {
    ParseSession& session;
    FunctionExtractor& extractor;
  };

  TranslationUnitScanState scan_state{
      .session = session,
      .extractor = extractor,
  };

  CXCursor root = clang_getTranslationUnitCursor(session.translation_unit());
  clang_visitChildren(
      root,
      [](CXCursor cursor, CXCursor /*parent*/, CXClientData data) {
        auto& state = *static_cast<TranslationUnitScanState*>(data);
        if (clang_getCursorKind(cursor) != CXCursor_FunctionDecl || !clang_isCursorDefinition(cursor)) {
          return CXChildVisit_Recurse;
        }
        if (!state.session.is_input_source_file(cursor) || !state.extractor.is_candidate_function(cursor)) {
          return CXChildVisit_Continue;
        }
        state.extractor.add_function(cursor);
        return CXChildVisit_Continue;
      },
      &scan_state);

  return session.take_facts();
}

}  // namespace entrypoint_codegen::clang_adapter
