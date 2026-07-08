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
  using diagnostics::Severity;

  diagnostic_set.add(diagnostics::Diagnostic{
      .code = code,
      .message = std::nullopt,
      .severity = Severity::kError,
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
  using diagnostics::DiagnosticCode;
  using facts::LayoutKind;
  using facts::ParamDecl;
  using facts::ParamRole;

  if (facts.string(function.return_type_spelling) != "void") {
    add_diagnostic(diagnostic_set, DiagnosticCode::kUnsupportedReturnType, function_id, function.location);
  }

  const auto params = facts.params(function.params);
  if (params.size() > 2) {
    add_diagnostic(diagnostic_set, DiagnosticCode::kUnsupportedParamCount, function_id, function.location);
  }

  bool uses_args = false;
  bool uses_res = false;
  for (size_t i = 0; i < params.size(); ++i) {
    const ParamDecl& param = params[i];
    if (!is_void_pointer_type(facts.string(param.type_spelling))) {
      add_diagnostic(diagnostic_set, DiagnosticCode::kUnsupportedParamType, function_id, param.location);
    }
    if (param.role == ParamRole::kOther) {
      add_diagnostic(diagnostic_set, DiagnosticCode::kUnknownParamRole, function_id, param.location);
      continue;
    }
    if (i == 0 && param.role == ParamRole::kRes && params.size() == 2) {
      add_diagnostic(diagnostic_set, DiagnosticCode::kInvalidTwoParamOrder, function_id, param.location);
    }
    if (i == 1 && param.role != ParamRole::kRes) {
      add_diagnostic(diagnostic_set, DiagnosticCode::kInvalidSecondParamRole, function_id, param.location);
    }
    uses_args |= param.role == ParamRole::kArgs;
    uses_res |= param.role == ParamRole::kRes;
  }

  const bool has_arguments = has_layout(facts, function, LayoutKind::kArguments);
  const bool has_result = has_layout(facts, function, LayoutKind::kResult);
  if (uses_args && !has_arguments) {
    add_diagnostic(diagnostic_set, DiagnosticCode::kMissingArgumentsLayout, function_id, function.location);
  }
  if (uses_res && !has_result) {
    add_diagnostic(diagnostic_set, DiagnosticCode::kMissingResultLayout, function_id, function.location);
  }
  if (!uses_args && has_arguments) {
    add_diagnostic(diagnostic_set, DiagnosticCode::kUnexpectedArgumentsLayout, function_id, function.location);
  }
  if (!uses_res && has_result) {
    add_diagnostic(diagnostic_set, DiagnosticCode::kUnexpectedResultLayout, function_id, function.location);
  }
}

}  // namespace

bool is_entrypoint_function_name(std::string_view name) {
  return name.starts_with(kFunctionNamePrefix);
}

bool is_void_pointer_type(std::string_view type) {
  return type == "void *" || type == "void*";
}

facts::BridgeKind bridge_kind_for_annotation(std::string_view annotation) {
  using facts::BridgeKind;

  if (annotation == kCGoAnnotation) {
    return BridgeKind::kCGo;
  }
  if (annotation == kFastCGoAnnotation) {
    return BridgeKind::kFastCGo;
  }
  return BridgeKind::kUnknown;
}

facts::ParamRole param_role_for_name(std::string_view name) {
  using facts::ParamRole;

  if (name == kArgsParamName) {
    return ParamRole::kArgs;
  }
  if (name == kResParamName) {
    return ParamRole::kRes;
  }
  return ParamRole::kOther;
}

std::optional<facts::LayoutKind> layout_kind_for_name(std::string_view name) {
  using facts::LayoutKind;

  if (name == kArgumentsLayoutName) {
    return LayoutKind::kArguments;
  }
  if (name == kResultLayoutName) {
    return LayoutKind::kResult;
  }
  return std::nullopt;
}

void validate_contract(const facts::FactArena& facts, diagnostics::DiagnosticSet& diagnostic_set) {
  using diagnostics::DiagnosticCode;
  using facts::BridgeKind;
  using facts::FunctionDecl;
  using facts::FunctionId;

  const auto functions = facts.functions();
  for (size_t i = 0; i < functions.size(); ++i) {
    const auto function_id = FunctionId(static_cast<uint32_t>(i));
    const FunctionDecl& function = functions[i];
    const std::string_view name = facts.string(function.name);
    if (!is_entrypoint_function_name(name)) {
      add_diagnostic(diagnostic_set, DiagnosticCode::kMissingNamePrefix, function_id, function.location);
    }
    if (!function.has_c_linkage) {
      add_diagnostic(diagnostic_set, DiagnosticCode::kMissingCLinkage, function_id, function.location);
    }
    if (function.bridge_kind == BridgeKind::kUnknown) {
      add_diagnostic(diagnostic_set, DiagnosticCode::kMissingEntrypointAttribute, function_id, function.location);
      continue;
    }
    if (function.bridge_kind == BridgeKind::kFastCGo) {
      validate_fastcgo_function(facts, diagnostic_set, function_id, function);
    }
  }
}

}  // namespace epgen::contract
