#pragma once

#include "app/options.h"

namespace epgen::diagnostics {
class DiagnosticSet;
}

namespace epgen::facts {
class FactStore;
}

namespace epgen::app {

void write_optional_output(const OutputOptions& options, const facts::FactStore& facts, const diagnostics::DiagnosticSet& diagnostic_set);

}  // namespace epgen::app
