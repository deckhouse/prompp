#include "app/run.h"

#include "app/runtime_debug.h"
#include "clang_adapter/parse.h"
#include "diagnostics/diagnostics.h"
#include "emit/diagnostics.h"
#include "emit/json.h"
#include "validate/validate.h"

#include <filesystem>
#include <fstream>
#include <iostream>
#include <memory_resource>
#include <stdexcept>

namespace entrypoint_codegen::app {

namespace {

class tracking_memory_resource : public std::pmr::memory_resource {
 public:
  explicit tracking_memory_resource(std::pmr::memory_resource* upstream = std::pmr::get_default_resource()) : upstream_(upstream) {}

  [[nodiscard]] size_t allocated_bytes() const noexcept { return allocated_bytes_; }
  [[nodiscard]] size_t deallocated_bytes() const noexcept { return deallocated_bytes_; }
  [[nodiscard]] size_t peak_live_bytes() const noexcept { return peak_live_bytes_; }

 private:
  void* do_allocate(size_t bytes, size_t alignment) override {
    void* result = upstream_->allocate(bytes, alignment);
    allocated_bytes_ += bytes;
    live_bytes_ += bytes;
    if (live_bytes_ > peak_live_bytes_) {
      peak_live_bytes_ = live_bytes_;
    }
    return result;
  }

  void do_deallocate(void* pointer, size_t bytes, size_t alignment) override {
    upstream_->deallocate(pointer, bytes, alignment);
    deallocated_bytes_ += bytes;
    live_bytes_ -= bytes;
  }

  bool do_is_equal(const std::pmr::memory_resource& other) const noexcept override { return this == &other; }

  std::pmr::memory_resource* upstream_;
  size_t allocated_bytes_ = 0;
  size_t deallocated_bytes_ = 0;
  size_t live_bytes_ = 0;
  size_t peak_live_bytes_ = 0;
};

struct DiagnosticCounts {
  uint32_t total = 0;
  uint32_t errors = 0;
  uint32_t warnings = 0;
  uint32_t infos = 0;
};

DiagnosticCounts count_diagnostics(const diagnostics::DiagnosticSet& diagnostic_set) {
  DiagnosticCounts counts;
  for (const diagnostics::Diagnostic& diagnostic : diagnostic_set.diagnostics()) {
    ++counts.total;
    switch (diagnostic.severity) {
      case diagnostics::Severity::kInfo: {
        ++counts.infos;
        break;
      }
      case diagnostics::Severity::kWarning: {
        ++counts.warnings;
        break;
      }
      case diagnostics::Severity::kError: {
        ++counts.errors;
        break;
      }
    }
  }
  return counts;
}

void write_json_output(const RunOptions& options, const facts::EntrypointFacts& facts, const diagnostics::DiagnosticSet& diagnostic_set) {
  if (options.output_path.has_parent_path()) {
    std::filesystem::create_directories(options.output_path.parent_path());
  }

  std::ofstream output(options.output_path, std::ios::trunc);
  if (!output) {
    throw std::runtime_error("failed to open output file: " + options.output_path.string());
  }
  emit::write_json(output, facts, diagnostic_set);
}

void write_lint_output(const RunOptions& options, const facts::EntrypointFacts& facts, const diagnostics::DiagnosticSet& diagnostic_set) {
  std::ostream& output = options.diagnostics_output == nullptr ? std::cout : *options.diagnostics_output;
  emit::write_diagnostics(output, facts, diagnostic_set);
}

}  // namespace

RunReport run(const RunOptions& options) {
  tracking_memory_resource memory_resource;
  diagnostics::DiagnosticSet diagnostic_set(&memory_resource);
  facts::EntrypointFacts facts = clang_adapter::parse_files(
      clang_adapter::ParseOptions{
          .source_files = options.source_files,
          .clang_args = options.clang_args,
          .memory_resource = &memory_resource,
      },
      diagnostic_set);

  validate::validate_entrypoints(facts, diagnostic_set);

  if (options.runtime_debug) {
    append_runtime_debug_diagnostics(diagnostic_set, RuntimeDebugSnapshot{
                                                         .allocated_bytes = memory_resource.allocated_bytes(),
                                                         .deallocated_bytes = memory_resource.deallocated_bytes(),
                                                         .peak_live_bytes = memory_resource.peak_live_bytes(),
                                                     });
  }

  switch (options.output_mode) {
    case OutputMode::kJson: {
      write_json_output(options, facts, diagnostic_set);
      break;
    }
    case OutputMode::kLint: {
      write_lint_output(options, facts, diagnostic_set);
      break;
    }
    case OutputMode::kCheck: {
      break;
    }
  }

  const DiagnosticCounts diagnostic_counts = count_diagnostics(diagnostic_set);
  return RunReport{
      .decision = diagnostic_counts.errors == 0 ? ExitDecision::kSuccess : ExitDecision::kAnalysisFailed,
      .diagnostic_count = diagnostic_counts.total,
      .error_count = diagnostic_counts.errors,
      .warning_count = diagnostic_counts.warnings,
      .info_count = diagnostic_counts.infos,
      .app_allocated_bytes = memory_resource.allocated_bytes(),
      .app_deallocated_bytes = memory_resource.deallocated_bytes(),
      .app_peak_live_bytes = memory_resource.peak_live_bytes(),
  };
}

}  // namespace entrypoint_codegen::app
