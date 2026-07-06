#include "app/argparse.h"

#include <algorithm>
#include <filesystem>
#include <fstream>
#include <ostream>
#include <stdexcept>
#include <string>
#include <vector>

namespace entrypoint_codegen::app {

namespace {

bool has_cpp_extension(const std::filesystem::path& path) {
  const std::string extension = path.extension().string();
  return extension == ".cpp" || extension == ".cc" || extension == ".cxx";
}

std::vector<std::filesystem::path> collect_input_files(const std::vector<std::filesystem::path>& inputs) {
  std::vector<std::filesystem::path> files;
  for (const std::filesystem::path& input : inputs) {
    if (!std::filesystem::exists(input)) {
      throw std::runtime_error("input path does not exist: " + input.string());
    }
    if (std::filesystem::is_regular_file(input)) {
      if (has_cpp_extension(input)) {
        files.push_back(std::filesystem::absolute(input).lexically_normal());
      }
      continue;
    }
    if (std::filesystem::is_directory(input)) {
      for (const auto& entry : std::filesystem::recursive_directory_iterator(input)) {
        if (entry.is_regular_file() && has_cpp_extension(entry.path())) {
          files.push_back(std::filesystem::absolute(entry.path()).lexically_normal());
        }
      }
    }
  }

  std::sort(files.begin(), files.end());
  files.erase(std::unique(files.begin(), files.end()), files.end());
  return files;
}

void append_clang_args_file(const std::filesystem::path& path, std::vector<std::string>& clang_args) {
  std::ifstream file(path);
  if (!file) {
    throw std::runtime_error("unable to open clang args file: " + path.string());
  }
  for (std::string line; std::getline(file, line);) {
    if (!line.empty()) {
      clang_args.push_back(std::move(line));
    }
  }
}

OutputMode parse_output_mode(std::string_view value) {
  if (value == "json") {
    return OutputMode::kJson;
  }
  if (value == "lint") {
    return OutputMode::kLint;
  }
  if (value == "check" || value == "none") {
    return OutputMode::kCheck;
  }
  throw std::runtime_error("unknown output mode: " + std::string(value));
}

}  // namespace

void write_help(std::ostream& out) {
  out << "entrypoint_codegen [options] <file_or_dir> [...] -- <clang_arg> [...]\n";
  out << "  --source=PATH           Additional source path, file or recursive directory.\n";
  out << "  --input=PATH            Alias for --source=PATH.\n";
  out << "  --output=PATH           JSON output path. Defaults to ./entrypoint_facts.json.\n";
  out << "  --output-dir=PATH       Directory for entrypoint_facts.json.\n";
  out << "  --mode=json|lint|check  Output mode. Defaults to json.\n";
  out << "  --format=json|lint|none Alias for --mode; none means check.\n";
  out << "  --no-output             Alias for --mode=check.\n";
  out << "  --runtime-debug         Append runtime debug diagnostics.\n";
  out << "  --clang-arg=ARG         Additional clang parser argument, repeatable.\n";
  out << "  --extra-arg=ARG         Alias for --clang-arg=ARG.\n";
  out << "  --clang-args-file=PATH  File with one clang parser argument per line.\n";
  out << "  --                      Treat remaining arguments as clang parser arguments.\n";
}

CliOptions parse_arguments(int argc, char** argv) {
  std::vector<std::filesystem::path> inputs;
  CliOptions options;
  options.run_options.output_path = std::filesystem::current_path() / "entrypoint_facts.json";

  for (int i = 1; i < argc; ++i) {
    const std::string arg = argv[i];
    if (arg == "--help" || arg == "-h") {
      options.help = true;
      return options;
    }
    if (arg.rfind("--output=", 0) == 0) {
      options.run_options.output_path = arg.substr(std::string("--output=").size());
      continue;
    }
    if (arg.rfind("--output-dir=", 0) == 0) {
      options.run_options.output_path = std::filesystem::path(arg.substr(std::string("--output-dir=").size())) / "entrypoint_facts.json";
      continue;
    }
    if (arg.rfind("--mode=", 0) == 0) {
      options.run_options.output_mode = parse_output_mode(arg.substr(std::string("--mode=").size()));
      continue;
    }
    if (arg.rfind("--format=", 0) == 0) {
      options.run_options.output_mode = parse_output_mode(arg.substr(std::string("--format=").size()));
      continue;
    }
    if (arg == "--no-output") {
      options.run_options.output_mode = OutputMode::kCheck;
      continue;
    }
    if (arg == "--runtime-debug") {
      options.run_options.runtime_debug = true;
      continue;
    }
    if (arg.rfind("--clang-arg=", 0) == 0) {
      options.run_options.clang_args.push_back(arg.substr(std::string("--clang-arg=").size()));
      continue;
    }
    if (arg.rfind("--extra-arg=", 0) == 0) {
      options.run_options.clang_args.push_back(arg.substr(std::string("--extra-arg=").size()));
      continue;
    }
    if (arg.rfind("--clang-args-file=", 0) == 0) {
      append_clang_args_file(arg.substr(std::string("--clang-args-file=").size()), options.run_options.clang_args);
      continue;
    }
    if (arg.rfind("--source=", 0) == 0) {
      inputs.emplace_back(arg.substr(std::string("--source=").size()));
      continue;
    }
    if (arg.rfind("--input=", 0) == 0) {
      inputs.emplace_back(arg.substr(std::string("--input=").size()));
      continue;
    }
    if (arg == "--") {
      for (++i; i < argc; ++i) {
        options.run_options.clang_args.emplace_back(argv[i]);
      }
      break;
    }
    inputs.emplace_back(arg);
  }

  options.run_options.source_files = collect_input_files(inputs);
  return options;
}

}  // namespace entrypoint_codegen::app
