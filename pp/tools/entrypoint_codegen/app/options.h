#pragma once

#include <cstdint>
#include <filesystem>
#include <iosfwd>
#include <string>
#include <vector>

namespace epgen::app {

enum class OutputMode : uint8_t {
  kJson,
  kLint,
  kCheck,
};

struct AnalysisOptions {
  std::vector<std::filesystem::path> source_files;
  std::vector<std::string> clang_args;
};

struct OutputOptions {
  std::filesystem::path output_path;
  OutputMode output_mode = OutputMode::kJson;
  std::ostream* diagnostics_output = nullptr;
};

struct RuntimeOptions {
  bool debug_diagnostics = false;
};

struct RunOptions {
  AnalysisOptions analysis;
  OutputOptions output;
  RuntimeOptions runtime;
};

enum class ExitDecision : uint8_t {
  kSuccess,
  kAnalysisFailed,
};

}  // namespace epgen::app
