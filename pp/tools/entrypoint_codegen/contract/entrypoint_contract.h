#pragma once

#include "facts/facts.h"

#include <optional>
#include <string_view>

namespace epgen::diagnostics {
class DiagnosticSet;
}

namespace epgen::facts {
class FactStore;
}

namespace epgen::contract {

constexpr std::string_view kFunctionNamePrefix = "prompp_";
constexpr std::string_view kCGoAnnotation = "prompp.entrypoint.cgo";
constexpr std::string_view kFastCGoAnnotation = "prompp.entrypoint.fastcgo";
constexpr std::string_view kArgumentsLayoutName = "Arguments";
constexpr std::string_view kResultLayoutName = "Result";
constexpr std::string_view kArgsParamName = "args";
constexpr std::string_view kResParamName = "res";

[[nodiscard]] bool is_entrypoint_function_name(std::string_view name);
[[nodiscard]] bool is_void_pointer_type(std::string_view type);

[[nodiscard]] facts::BridgeKind bridge_kind_for_annotation(std::string_view annotation);
[[nodiscard]] facts::ParamRole param_role_for_name(std::string_view name);
[[nodiscard]] std::optional<facts::LayoutKind> layout_kind_for_name(std::string_view name);

void validate_contract(const facts::FactStore& facts, diagnostics::DiagnosticSet& diagnostic_set);

}  // namespace epgen::contract
