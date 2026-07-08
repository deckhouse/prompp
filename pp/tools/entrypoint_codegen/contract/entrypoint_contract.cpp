#include "contract/entrypoint_contract.h"

#include "diagnostics/diagnostics.h"
#include "facts/fact_arena.h"

#include <cstddef>
#include <cstdint>

namespace epgen::contract {

namespace {

void add_diagnostic(diagnostics::DiagnosticSet& diagnostic_set,
                    diagnostics::DiagnosticCode code,
                    facts::FunctionId function_id,
                    facts::SourceLocation location) {
  diagnostic_set.add(diagnostics::Diagnostic{
      .code = code,
      .message = std::nullopt,
      .severity = diagnostics::Severity::kError,
      .function = function_id,
      .location = location,
  });
}

bool has_layout(const facts::FactArena& facts, const facts::FunctionDecl& function, facts::LayoutKind kind) {
  for (const facts::LayoutDecl& layout : facts.layouts(function.layouts)) {
    if (layout.kind == kind) {
      return true;
    }
  }
  return false;
}

void validate_fastcgo_function(const facts::FactArena& facts,
                               diagnostics::DiagnosticSet& diagnostic_set,
                               facts::FunctionId function_id,
                               const facts::FunctionDecl& function) {
  if (facts.string(function.return_type_spelling) != "void") {
    add_diagnostic(diagnostic_set, diagnostics::DiagnosticCode::kUnsupportedReturnType, function_id, function.location);
  }

  const auto params = facts.params(function.params);
  if (params.size() > 2) {
    add_diagnostic(diagnostic_set, diagnostics::DiagnosticCode::kUnsupportedParamCount, function_id, function.location);
  }

  bool uses_args = false;
  bool uses_res = false;
  for (size_t i = 0; i < params.size(); ++i) {
    const facts::ParamDecl& param = params[i];
    if (!is_void_pointer_type(facts.string(param.type_spelling))) {
      add_diagnostic(diagnostic_set, diagnostics::DiagnosticCode::kUnsupportedParamType, function_id, param.location);
    }
    if (param.role == facts::ParamRole::kOther) {
      add_diagnostic(diagnostic_set, diagnostics::DiagnosticCode::kUnknownParamRole, function_id, param.location);
      continue;
    }
    if (i == 0 && param.role == facts::ParamRole::kRes && params.size() == 2) {
      add_diagnostic(diagnostic_set, diagnostics::DiagnosticCode::kInvalidTwoParamOrder, function_id, param.location);
    }
    if (i == 1 && param.role != facts::ParamRole::kRes) {
      add_diagnostic(diagnostic_set, diagnostics::DiagnosticCode::kInvalidSecondParamRole, function_id, param.location);
    }
    uses_args |= param.role == facts::ParamRole::kArgs;
    uses_res |= param.role == facts::ParamRole::kRes;
  }

  const bool has_arguments = has_layout(facts, function, facts::LayoutKind::kArguments);
  const bool has_result = has_layout(facts, function, facts::LayoutKind::kResult);
  if (uses_args && !has_arguments) {
    add_diagnostic(diagnostic_set, diagnostics::DiagnosticCode::kMissingArgumentsLayout, function_id, function.location);
  }
  if (uses_res && !has_result) {
    add_diagnostic(diagnostic_set, diagnostics::DiagnosticCode::kMissingResultLayout, function_id, function.location);
  }
  if (!uses_args && has_arguments) {
    add_diagnostic(diagnostic_set, diagnostics::DiagnosticCode::kUnexpectedArgumentsLayout, function_id, function.location);
  }
  if (!uses_res && has_result) {
    add_diagnostic(diagnostic_set, diagnostics::DiagnosticCode::kUnexpectedResultLayout, function_id, function.location);
  }
}

}  // namespace

bool starts_with(std::string_view value, std::string_view prefix) {
  return value.substr(0, prefix.size()) == prefix;
}

bool is_entrypoint_function_name(std::string_view name) {
  return starts_with(name, kFunctionNamePrefix);
}

bool is_void_pointer_type(std::string_view type) {
  return type == "void *" || type == "void*";
}

facts::BridgeKind bridge_kind_for_annotation(std::string_view annotation) {
  if (annotation == kCGoAnnotation) {
    return facts::BridgeKind::kCGo;
  }
  if (annotation == kFastCGoAnnotation) {
    return facts::BridgeKind::kFastCGo;
  }
  return facts::BridgeKind::kUnknown;
}

facts::ParamRole param_role_for_name(std::string_view name) {
  if (name == kArgsParamName) {
    return facts::ParamRole::kArgs;
  }
  if (name == kResParamName) {
    return facts::ParamRole::kRes;
  }
  return facts::ParamRole::kOther;
}

std::optional<facts::LayoutKind> layout_kind_for_name(std::string_view name) {
  if (name == kArgumentsLayoutName) {
    return facts::LayoutKind::kArguments;
  }
  if (name == kResultLayoutName) {
    return facts::LayoutKind::kResult;
  }
  return std::nullopt;
}

void validate_entrypoints(const facts::FactArena& facts, diagnostics::DiagnosticSet& diagnostic_set) {
  const auto functions = facts.functions();
  for (size_t i = 0; i < functions.size(); ++i) {
    const auto function_id = facts::FunctionId(static_cast<uint32_t>(i));
    const facts::FunctionDecl& function = functions[i];
    const std::string_view name = facts.string(function.name);
    if (!is_entrypoint_function_name(name)) {
      add_diagnostic(diagnostic_set, diagnostics::DiagnosticCode::kMissingNamePrefix, function_id, function.location);
    }
    if (!function.has_c_linkage) {
      add_diagnostic(diagnostic_set, diagnostics::DiagnosticCode::kMissingCLinkage, function_id, function.location);
    }
    if (function.bridge_kind == facts::BridgeKind::kUnknown) {
      add_diagnostic(diagnostic_set, diagnostics::DiagnosticCode::kMissingEntrypointAttribute, function_id, function.location);
      continue;
    }
    if (function.bridge_kind == facts::BridgeKind::kFastCGo) {
      validate_fastcgo_function(facts, diagnostic_set, function_id, function);
    }
  }
}

}  // namespace epgen::contract
