#pragma once

#include "facts/entrypoint_facts.h"

#include <iosfwd>

namespace entrypoint_codegen::emit {

void write_diagnostics(std::ostream& out, const facts::EntrypointFacts& facts);

}  // namespace entrypoint_codegen::emit
