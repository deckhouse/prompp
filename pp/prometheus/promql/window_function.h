#pragma once

#include <array>
#include <cstdint>
#include <string_view>

#include "prometheus/promql/function_names_hash.h"

namespace PromPP::Prometheus::promql {

enum class WindowFunction : uint8_t {
  kNone = 0,
  kRate,
  kIncrease,
  kIrate,
  kIdelta,
  kMinOverTime,
  kMaxOverTime,
  kLastOverTime,
  kLastOverStep,
  kSumOverTime,
  kCountOverTime,
  kDelta,
  kResets,
  kChanges,
  kSum,
  kMin,
  kMax,
};

enum class FunctionType : uint8_t {
  kNone = 0,
  kThinning,
  kSynthesizing,
  kCrossSeriesSynthesizing,
};

struct Function {
  std::string_view name;
  FunctionType type;
};

// The order of the functions must match the order in the WindowFunction enum
constexpr std::array kFunctions = {
    Function{.name = "", .type = FunctionType::kNone},
    Function{.name = "rate", .type = FunctionType::kThinning},
    Function{.name = "increase", .type = FunctionType::kThinning},
    Function{.name = "irate", .type = FunctionType::kThinning},
    Function{.name = "idelta", .type = FunctionType::kThinning},
    Function{.name = "min_over_time", .type = FunctionType::kThinning},
    Function{.name = "max_over_time", .type = FunctionType::kThinning},
    Function{.name = "last_over_time", .type = FunctionType::kThinning},
    Function{.name = "last_over_step", .type = FunctionType::kSynthesizing},
    Function{.name = "sum_over_time", .type = FunctionType::kSynthesizing},
    Function{.name = "count_over_time", .type = FunctionType::kSynthesizing},
    Function{.name = "delta", .type = FunctionType::kThinning},
    Function{.name = "resets", .type = FunctionType::kThinning},
    Function{.name = "changes", .type = FunctionType::kThinning},
    Function{.name = "sum", .type = FunctionType::kCrossSeriesSynthesizing},
    Function{.name = "min", .type = FunctionType::kCrossSeriesSynthesizing},
    Function{.name = "max", .type = FunctionType::kCrossSeriesSynthesizing},
};

constexpr uint32_t function_name_hash(std::string_view str) {
  return FunctionNamesHash::hash(str.data(), str.length());
}

[[nodiscard]] constexpr WindowFunction window_function_from_string(std::string_view name) noexcept {
  if (name.empty()) {
    return WindowFunction::kNone;
  }

  const auto hash = function_name_hash(name);

#define PROMQL_WINDOW_FUNC_CASE(kind)                                                       \
  case function_name_hash(kFunctions[static_cast<uint8_t>(WindowFunction::kind)].name): {   \
    if (name != kFunctions[static_cast<uint8_t>(WindowFunction::kind)].name) [[unlikely]] { \
      break;                                                                                \
    }                                                                                       \
    return WindowFunction::kind;                                                            \
  }

  switch (hash) {
    PROMQL_WINDOW_FUNC_CASE(kRate)
    PROMQL_WINDOW_FUNC_CASE(kIncrease)
    PROMQL_WINDOW_FUNC_CASE(kIrate)
    PROMQL_WINDOW_FUNC_CASE(kIdelta)
    PROMQL_WINDOW_FUNC_CASE(kMinOverTime)
    PROMQL_WINDOW_FUNC_CASE(kMaxOverTime)
    PROMQL_WINDOW_FUNC_CASE(kLastOverTime)
    PROMQL_WINDOW_FUNC_CASE(kLastOverStep)
    PROMQL_WINDOW_FUNC_CASE(kSumOverTime)
    PROMQL_WINDOW_FUNC_CASE(kCountOverTime)
    PROMQL_WINDOW_FUNC_CASE(kDelta)
    PROMQL_WINDOW_FUNC_CASE(kResets)
    PROMQL_WINDOW_FUNC_CASE(kChanges)
    PROMQL_WINDOW_FUNC_CASE(kSum)
    PROMQL_WINDOW_FUNC_CASE(kMax)
    PROMQL_WINDOW_FUNC_CASE(kMin)
    default:
      return WindowFunction::kNone;
  }

#undef PROMQL_WINDOW_FUNC_CASE

  return WindowFunction::kNone;
}

}  // namespace PromPP::Prometheus::promql