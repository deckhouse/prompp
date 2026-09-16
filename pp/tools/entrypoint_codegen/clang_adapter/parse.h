#pragma once

#include "clang_adapter/parse_options.h"
#include "diagnostics/diagnostics.h"
#include "facts/fact_store.h"

namespace epgen::clang_adapter {

facts::FactStore parse_files(const ParseOptions& options, diagnostics::DiagnosticSet& diagnostic_set);

}  // namespace epgen::clang_adapter
