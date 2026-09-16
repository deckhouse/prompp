#include "app/output.h"

#include "diagnostics/diagnostics.h"
#include "emit/report.h"
#include "facts/fact_store.h"

#include <filesystem>
#include <fstream>
#include <iostream>
#include <stdexcept>

namespace epgen::app {

namespace {

void write_json_output(const OutputOptions& options, const facts::FactStore& facts, const diagnostics::DiagnosticSet& diagnostic_set) {
  if (options.output_path.has_parent_path()) {
    std::filesystem::create_directories(options.output_path.parent_path());
  }

  std::ofstream output(options.output_path, std::ios::trunc);
  if (!output) {
    throw std::runtime_error("failed to open output file: " + options.output_path.string());
  }
  emit::write_report(output, emit::ReportFormat::kJson, facts, diagnostic_set);
}

void write_lint_output(const OutputOptions& options, const facts::FactStore& facts, const diagnostics::DiagnosticSet& diagnostic_set) {
  std::ostream& output = options.diagnostics_output == nullptr ? std::cout : *options.diagnostics_output;
  emit::write_report(output, emit::ReportFormat::kCompilerDiagnostics, facts, diagnostic_set);
}

}  // namespace

void write_optional_output(const OutputOptions& options, const facts::FactStore& facts, const diagnostics::DiagnosticSet& diagnostic_set) {
  switch (options.output_mode) {
    case OutputMode::kJson: {
      write_json_output(options, facts, diagnostic_set);
      break;
    }
    case OutputMode::kLint: {
      write_lint_output(options, facts, diagnostic_set);
      break;
    }
  }
}

}  // namespace epgen::app
