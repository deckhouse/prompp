#include "contract/entrypoint_contract.h"

namespace entrypoint_codegen::contract {

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

}  // namespace entrypoint_codegen::contract
