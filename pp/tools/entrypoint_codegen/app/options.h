#pragma once

#include <cstddef>
#include <cstdint>
#include <filesystem>
#include <iosfwd>
#include <string>
#include <vector>

namespace entrypoint_codegen::app {

enum class OutputMode : uint8_t {
  kJson,
  kLint,
  kCheck,
};

struct RunOptions {
  std::vector<std::filesystem::path> source_files;
  std::vector<std::string> clang_args;
  std::filesystem::path output_path;
  OutputMode output_mode = OutputMode::kJson;
  bool runtime_debug = false;
  std::ostream* diagnostics_output = nullptr;
};

enum class ExitDecision : uint8_t {
  kSuccess,
  kAnalysisFailed,
};

struct RunReport {
  ExitDecision decision;
  uint32_t diagnostic_count;
  uint32_t error_count;
  uint32_t warning_count;
  uint32_t info_count;
  size_t app_allocated_bytes;
  size_t app_deallocated_bytes;
  size_t app_peak_live_bytes;
};

}  // namespace entrypoint_codegen::app
