#pragma once

#include "clang_adapter/parse_options.h"
#include "diagnostics/diagnostics.h"
#include "facts/fact_arena.h"

namespace epgen::clang_adapter {

facts::FactArena parse_files(const ParseOptions& options, diagnostics::DiagnosticSet& diagnostic_set);

}  // namespace epgen::clang_adapter
