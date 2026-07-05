#pragma once

#include "app/run.h"

#include <iosfwd>

namespace entrypoint_codegen::app {

struct CliOptions {
  RunOptions run_options;
  bool help = false;
};

CliOptions parse_arguments(int argc, char** argv);
void write_help(std::ostream& out);

}  // namespace entrypoint_codegen::app
