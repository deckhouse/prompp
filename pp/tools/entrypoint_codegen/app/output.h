#pragma once

#include "app/options.h"

namespace epgen::diagnostics {
class DiagnosticSet;
}

namespace epgen::facts {
class FactArena;
}

namespace epgen::app {

void write_optional_output(const OutputOptions& options, const facts::FactArena& facts, const diagnostics::DiagnosticSet& diagnostic_set);

}  // namespace epgen::app
