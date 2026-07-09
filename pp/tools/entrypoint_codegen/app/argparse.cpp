#include "app/argparse.h"

#include <algorithm>
#include <filesystem>
#include <ostream>
#include <stdexcept>
#include <string>
#include <vector>

namespace epgen::app {

namespace {

bool has_cpp_extension(const std::filesystem::path& path) {
  const std::string extension = path.extension().string();
  return extension == ".cpp" || extension == ".cc" || extension == ".cxx";
}

OutputMode parse_output_mode(std::string_view value) {
  if (value == "json") {
    return OutputMode::kJson;
  }
  if (value == "lint") {
    return OutputMode::kLint;
  }
  throw std::runtime_error("unknown output mode: " + std::string(value));
}

std::vector<std::filesystem::path> collect_source_files(const std::vector<std::filesystem::path>& inputs) {
  std::vector<std::filesystem::path> files;
  for (const std::filesystem::path& input : inputs) {
    if (!std::filesystem::exists(input)) {
      throw std::runtime_error("input path does not exist: " + input.string());
    }
    if (!std::filesystem::is_regular_file(input)) {
      throw std::runtime_error("input path is not a source file: " + input.string());
    }
    if (!has_cpp_extension(input)) {
      throw std::runtime_error("input path is not a supported C++ source file: " + input.string());
    }
    files.push_back(std::filesystem::absolute(input).lexically_normal());
  }

  std::sort(files.begin(), files.end());
  files.erase(std::unique(files.begin(), files.end()), files.end());
  return files;
}

}  // namespace

void write_help(std::ostream& out) {
  out << "entrypoint_codegen [options] <source_file> [...] -- <clang_arg> [...]\n";
  out << "  --mode=json|lint        Output mode. Defaults to json.\n";
  out << "  --output=PATH           Required JSON output path when mode is json.\n";
  out << "  --runtime-debug         Append runtime debug diagnostics.\n";
  out << "  --                      Treat remaining arguments as clang parser arguments.\n";
}

CliOptions parse_arguments(int argc, char** argv) {
  std::vector<std::filesystem::path> inputs;
  CliOptions options;

  for (int i = 1; i < argc; ++i) {
    const std::string arg = argv[i];
    if (arg == "--help" || arg == "-h") {
      options.help = true;
      return options;
    }
    if (arg.rfind("--output=", 0) == 0) {
      options.run_options.output.output_path = arg.substr(std::string("--output=").size());
      continue;
    }
    if (arg.rfind("--mode=", 0) == 0) {
      options.run_options.output.output_mode = parse_output_mode(arg.substr(std::string("--mode=").size()));
      continue;
    }
    if (arg == "--runtime-debug") {
      options.run_options.runtime.debug_diagnostics = true;
      continue;
    }
    if (arg == "--") {
      for (++i; i < argc; ++i) {
        options.run_options.analysis.clang_args.emplace_back(argv[i]);
      }
      break;
    }
    if (arg.rfind("--", 0) == 0) {
      throw std::runtime_error("unknown option: " + arg);
    }
    inputs.emplace_back(arg);
  }

  options.run_options.analysis.source_files = collect_source_files(inputs);
  if (options.run_options.output.output_mode == OutputMode::kJson && !options.run_options.analysis.source_files.empty() &&
      options.run_options.output.output_path.empty()) {
    throw std::runtime_error("missing required --output for json mode");
  }
  return options;
}

}  // namespace epgen::app
