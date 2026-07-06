#include "app/argparse.h"
#include "app/run.h"

#include <exception>
#include <iostream>

int main(int argc, char** argv) {
  namespace app = entrypoint_codegen::app;

  app::CliOptions cli_options;
  try {
    cli_options = app::parse_arguments(argc, argv);
  } catch (const std::exception& error) {
    std::cerr << "entrypoint_codegen failed: " << error.what() << "\n";
    return 1;
  }

  if (cli_options.help) {
    app::write_help(std::cout);
    return 0;
  }

  if (cli_options.run_options.analysis.source_files.empty()) {
    app::write_help(std::cout);
    return 1;
  }

  cli_options.run_options.output.diagnostics_output = &std::cout;

  app::RunReport report;
  try {
    report = app::run(cli_options.run_options);
  } catch (const std::exception& error) {
    std::cerr << "entrypoint_codegen failed: " << error.what() << "\n";
    return 1;
  }

  if (cli_options.run_options.output.output_mode == app::OutputMode::kJson) {
    std::cout << "Wrote " << cli_options.run_options.output.output_path << "\n";
  }

  if (report.decision == app::ExitDecision::kAnalysisFailed) {
    std::cerr << "entrypoint_codegen found " << report.diagnostics.errors << " error diagnostics";
    if (report.diagnostics.warnings != 0 || report.diagnostics.infos != 0) {
      std::cerr << " (" << report.diagnostics.warnings << " warnings, " << report.diagnostics.infos << " info)";
    }
    std::cerr << "\n";
    return 2;
  }
  return 0;
}
