#include "emit/diagnostic_text.h"

namespace entrypoint_codegen::emit {

std::string_view diagnostic_code_name(facts::DiagnosticCode code) {
  switch (code) {
    case facts::DiagnosticCode::kClangDiagnostic: {
      return "clang_diagnostic";
    }
    case facts::DiagnosticCode::kUnsupportedReturnType: {
      return "unsupported_return_type";
    }
    case facts::DiagnosticCode::kUnsupportedParamCount: {
      return "unsupported_param_count";
    }
    case facts::DiagnosticCode::kUnsupportedParamType: {
      return "unsupported_param_type";
    }
    case facts::DiagnosticCode::kUnknownParamRole:
    case facts::DiagnosticCode::kInvalidTwoParamOrder:
    case facts::DiagnosticCode::kInvalidSecondParamRole: {
      return "unknown_param_role";
    }
    case facts::DiagnosticCode::kMissingArgumentsLayout: {
      return "missing_arguments_layout";
    }
    case facts::DiagnosticCode::kMissingResultLayout: {
      return "missing_result_layout";
    }
    case facts::DiagnosticCode::kUnexpectedArgumentsLayout: {
      return "unexpected_arguments_layout";
    }
    case facts::DiagnosticCode::kUnexpectedResultLayout: {
      return "unexpected_result_layout";
    }
    case facts::DiagnosticCode::kMissingNamePrefix: {
      return "missing_name_prefix";
    }
    case facts::DiagnosticCode::kMissingCLinkage: {
      return "missing_c_linkage";
    }
    case facts::DiagnosticCode::kMissingEntrypointAttribute: {
      return "missing_entrypoint_attribute";
    }
    case facts::DiagnosticCode::kRuntimeMemoryUsage: {
      return "runtime_memory_usage";
    }
  }
  return "unknown_diagnostic";
}

std::string_view diagnostic_default_message(facts::DiagnosticCode code) {
  switch (code) {
    case facts::DiagnosticCode::kClangDiagnostic: {
      return "Clang diagnostic";
    }
    case facts::DiagnosticCode::kUnsupportedReturnType: {
      return "FastCGo entrypoint must return void";
    }
    case facts::DiagnosticCode::kUnsupportedParamCount: {
      return "FastCGo entrypoint supports 0, 1, or 2 parameters";
    }
    case facts::DiagnosticCode::kUnsupportedParamType: {
      return "FastCGo entrypoint parameters must be void*";
    }
    case facts::DiagnosticCode::kUnknownParamRole: {
      return "FastCGo parameter must be named args or res";
    }
    case facts::DiagnosticCode::kInvalidTwoParamOrder: {
      return "FastCGo two-parameter form must be args, res";
    }
    case facts::DiagnosticCode::kInvalidSecondParamRole: {
      return "FastCGo second parameter must be res";
    }
    case facts::DiagnosticCode::kMissingArgumentsLayout: {
      return "args parameter requires local Arguments layout";
    }
    case facts::DiagnosticCode::kMissingResultLayout: {
      return "res parameter requires local Result layout";
    }
    case facts::DiagnosticCode::kUnexpectedArgumentsLayout: {
      return "Arguments layout exists but args parameter is absent";
    }
    case facts::DiagnosticCode::kUnexpectedResultLayout: {
      return "Result layout exists but res parameter is absent";
    }
    case facts::DiagnosticCode::kMissingNamePrefix: {
      return "entrypoint function must use prompp_ prefix";
    }
    case facts::DiagnosticCode::kMissingCLinkage: {
      return "entrypoint function must be declared extern \"C\"";
    }
    case facts::DiagnosticCode::kMissingEntrypointAttribute: {
      return "entrypoint function requires CGo or FastCGo annotation";
    }
    case facts::DiagnosticCode::kRuntimeMemoryUsage: {
      return "runtime memory usage";
    }
  }
  return "unknown diagnostic";
}

std::string_view diagnostic_message(const facts::EntrypointFacts& facts, const facts::Diagnostic& diagnostic) {
  if (diagnostic.message.has_value()) {
    return facts.string(*diagnostic.message);
  }
  return diagnostic_default_message(diagnostic.code);
}

}  // namespace entrypoint_codegen::emit
