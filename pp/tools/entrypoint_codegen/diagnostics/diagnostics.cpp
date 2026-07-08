#include "diagnostics/diagnostics.h"

#include <utility>

namespace epgen::diagnostics {

bool SeverityCounts::has_errors() const noexcept {
  return errors != 0;
}

DiagnosticSet::DiagnosticSet(std::pmr::memory_resource* memory_resource) : diagnostics_(memory_resource) {}

void DiagnosticSet::add(Diagnostic diagnostic) {
  diagnostics_.push_back(std::move(diagnostic));
}

std::span<const Diagnostic> DiagnosticSet::diagnostics() const noexcept {
  return diagnostics_;
}

uint32_t DiagnosticSet::count() const noexcept {
  return static_cast<uint32_t>(diagnostics_.size());
}

bool DiagnosticSet::empty() const noexcept {
  return diagnostics_.empty();
}

SeverityCounts count_by_severity(const DiagnosticSet& diagnostic_set) {
  SeverityCounts counts;
  for (const Diagnostic& diagnostic : diagnostic_set.diagnostics()) {
    ++counts.total;
    switch (diagnostic.severity) {
      case Severity::kInfo: {
        ++counts.infos;
        break;
      }
      case Severity::kWarning: {
        ++counts.warnings;
        break;
      }
      case Severity::kError: {
        ++counts.errors;
        break;
      }
    }
  }
  return counts;
}

std::string_view diagnostic_code_name(DiagnosticCode code) {
  switch (code) {
    case DiagnosticCode::kClangDiagnostic: {
      return "clang_diagnostic";
    }
    case DiagnosticCode::kUnsupportedReturnType: {
      return "unsupported_return_type";
    }
    case DiagnosticCode::kUnsupportedParamCount: {
      return "unsupported_param_count";
    }
    case DiagnosticCode::kUnsupportedParamType: {
      return "unsupported_param_type";
    }
    case DiagnosticCode::kUnknownParamRole: {
      return "unknown_param_role";
    }
    case DiagnosticCode::kInvalidTwoParamOrder: {
      return "invalid_two_param_order";
    }
    case DiagnosticCode::kInvalidSecondParamRole: {
      return "invalid_second_param_role";
    }
    case DiagnosticCode::kMissingArgumentsLayout: {
      return "missing_arguments_layout";
    }
    case DiagnosticCode::kMissingResultLayout: {
      return "missing_result_layout";
    }
    case DiagnosticCode::kUnexpectedArgumentsLayout: {
      return "unexpected_arguments_layout";
    }
    case DiagnosticCode::kUnexpectedResultLayout: {
      return "unexpected_result_layout";
    }
    case DiagnosticCode::kMissingNamePrefix: {
      return "missing_name_prefix";
    }
    case DiagnosticCode::kMissingCLinkage: {
      return "missing_c_linkage";
    }
    case DiagnosticCode::kMissingEntrypointAttribute: {
      return "missing_entrypoint_attribute";
    }
    case DiagnosticCode::kRuntimeMemoryUsage: {
      return "runtime_memory_usage";
    }
  }
  return "unknown_diagnostic";
}

std::string_view diagnostic_default_message(DiagnosticCode code) {
  switch (code) {
    case DiagnosticCode::kClangDiagnostic: {
      return "Clang diagnostic";
    }
    case DiagnosticCode::kUnsupportedReturnType: {
      return "FastCGo entrypoint must return void";
    }
    case DiagnosticCode::kUnsupportedParamCount: {
      return "FastCGo entrypoint supports 0, 1, or 2 parameters";
    }
    case DiagnosticCode::kUnsupportedParamType: {
      return "FastCGo entrypoint parameters must be void*";
    }
    case DiagnosticCode::kUnknownParamRole: {
      return "FastCGo parameter must be named args or res";
    }
    case DiagnosticCode::kInvalidTwoParamOrder: {
      return "FastCGo two-parameter form must be args, res";
    }
    case DiagnosticCode::kInvalidSecondParamRole: {
      return "FastCGo second parameter must be res";
    }
    case DiagnosticCode::kMissingArgumentsLayout: {
      return "args parameter requires local Arguments layout";
    }
    case DiagnosticCode::kMissingResultLayout: {
      return "res parameter requires local Result layout";
    }
    case DiagnosticCode::kUnexpectedArgumentsLayout: {
      return "Arguments layout exists but args parameter is absent";
    }
    case DiagnosticCode::kUnexpectedResultLayout: {
      return "Result layout exists but res parameter is absent";
    }
    case DiagnosticCode::kMissingNamePrefix: {
      return "entrypoint function must use prompp_ prefix";
    }
    case DiagnosticCode::kMissingCLinkage: {
      return "entrypoint function must be declared extern \"C\"";
    }
    case DiagnosticCode::kMissingEntrypointAttribute: {
      return "entrypoint function requires CGo or FastCGo annotation";
    }
    case DiagnosticCode::kRuntimeMemoryUsage: {
      return "runtime memory usage";
    }
  }
  return "unknown diagnostic";
}

std::string_view diagnostic_message(const Diagnostic& diagnostic) {
  if (diagnostic.message.has_value()) {
    return *diagnostic.message;
  }
  return diagnostic_default_message(diagnostic.code);
}

std::string_view severity_name(Severity severity) {
  switch (severity) {
    case Severity::kInfo: {
      return "info";
    }
    case Severity::kWarning: {
      return "warning";
    }
    case Severity::kError: {
      return "error";
    }
  }
  return "error";
}

}  // namespace epgen::diagnostics
