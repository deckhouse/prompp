#include "validate/validate.h"

#include <optional>
#include <string_view>

namespace entrypoint_codegen::validate {

namespace {

bool is_void_pointer_type(std::string_view type) {
  return type == "void *" || type == "void*";
}

bool starts_with(std::string_view value, std::string_view prefix) {
  return value.substr(0, prefix.size()) == prefix;
}

void add_diagnostic(facts::EntrypointFacts& facts, facts::DiagnosticCode code, facts::FunctionId function_id, facts::SourceLocation location) {
  facts.add_diagnostic(facts::Diagnostic{
      .code = code,
      .message = std::nullopt,
      .severity = facts::Severity::kError,
      .function = function_id,
      .location = location,
  });
}

bool has_layout(const facts::EntrypointFacts& facts, const facts::FunctionDecl& function, facts::LayoutKind kind) {
  for (const facts::LayoutDecl& layout : facts.layouts(function.layouts)) {
    if (layout.kind == kind) {
      return true;
    }
  }
  return false;
}

void validate_fastcgo_function(facts::EntrypointFacts& facts, facts::FunctionId function_id, const facts::FunctionDecl& function) {
  if (facts.string(function.return_type_spelling) != "void") {
    add_diagnostic(facts, facts::DiagnosticCode::kUnsupportedReturnType, function_id, function.location);
  }

  const auto params = facts.params(function.params);
  if (params.size() > 2) {
    add_diagnostic(facts, facts::DiagnosticCode::kUnsupportedParamCount, function_id, function.location);
  }

  bool uses_args = false;
  bool uses_res = false;
  for (size_t i = 0; i < params.size(); ++i) {
    const facts::ParamDecl& param = params[i];
    if (!is_void_pointer_type(facts.string(param.type_spelling))) {
      add_diagnostic(facts, facts::DiagnosticCode::kUnsupportedParamType, function_id, param.location);
    }
    if (param.role == facts::ParamRole::kOther) {
      add_diagnostic(facts, facts::DiagnosticCode::kUnknownParamRole, function_id, param.location);
      continue;
    }
    if (i == 0 && param.role == facts::ParamRole::kRes && params.size() == 2) {
      add_diagnostic(facts, facts::DiagnosticCode::kInvalidTwoParamOrder, function_id, param.location);
    }
    if (i == 1 && param.role != facts::ParamRole::kRes) {
      add_diagnostic(facts, facts::DiagnosticCode::kInvalidSecondParamRole, function_id, param.location);
    }
    uses_args |= param.role == facts::ParamRole::kArgs;
    uses_res |= param.role == facts::ParamRole::kRes;
  }

  const bool has_arguments = has_layout(facts, function, facts::LayoutKind::kArguments);
  const bool has_result = has_layout(facts, function, facts::LayoutKind::kResult);
  if (uses_args && !has_arguments) {
    add_diagnostic(facts, facts::DiagnosticCode::kMissingArgumentsLayout, function_id, function.location);
  }
  if (uses_res && !has_result) {
    add_diagnostic(facts, facts::DiagnosticCode::kMissingResultLayout, function_id, function.location);
  }
  if (!uses_args && has_arguments) {
    add_diagnostic(facts, facts::DiagnosticCode::kUnexpectedArgumentsLayout, function_id, function.location);
  }
  if (!uses_res && has_result) {
    add_diagnostic(facts, facts::DiagnosticCode::kUnexpectedResultLayout, function_id, function.location);
  }
}

}  // namespace

void validate_entrypoints(facts::EntrypointFacts& facts) {
  const auto functions = facts.functions();
  for (size_t i = 0; i < functions.size(); ++i) {
    const auto function_id = facts::FunctionId(static_cast<uint32_t>(i));
    const facts::FunctionDecl& function = functions[i];
    const std::string_view name = facts.string(function.name);
    if (!starts_with(name, "prompp_")) {
      add_diagnostic(facts, facts::DiagnosticCode::kMissingNamePrefix, function_id, function.location);
    }
    if (!function.has_c_linkage) {
      add_diagnostic(facts, facts::DiagnosticCode::kMissingCLinkage, function_id, function.location);
    }
    if (function.bridge_kind == facts::BridgeKind::kUnknown) {
      add_diagnostic(facts, facts::DiagnosticCode::kMissingEntrypointAttribute, function_id, function.location);
      continue;
    }
    if (function.bridge_kind == facts::BridgeKind::kFastCGo) {
      validate_fastcgo_function(facts, function_id, function);
    }
  }
}

}  // namespace entrypoint_codegen::validate
