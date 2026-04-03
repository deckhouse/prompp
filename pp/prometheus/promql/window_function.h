#pragma once

#include <cstdint>
#include <string_view>

#include "bare_bones/preprocess.h"
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
  kSumOverTime,
  kDelta,
  kResets,
  kChanges,
};

constexpr uint32_t function_name_hash(std::string_view str) {
  return FunctionNamesHash::hash(str.data(), str.length());
}

[[nodiscard]] PROMPP_ALWAYS_INLINE WindowFunction window_function_from_string(std::string_view name) noexcept {
  if (name.empty()) {
    return WindowFunction::kNone;
  }

  const auto hash = function_name_hash(name);

#define PROMQL_WINDOW_FUNC_CASE(literal, kind) \
  case function_name_hash(literal): {          \
    if (name != literal) [[unlikely]] {        \
      break;                                   \
    }                                          \
    return WindowFunction::kind;               \
  }

  switch (hash) {
    PROMQL_WINDOW_FUNC_CASE("rate", kRate)
    PROMQL_WINDOW_FUNC_CASE("increase", kIncrease)
    PROMQL_WINDOW_FUNC_CASE("irate", kIrate)
    PROMQL_WINDOW_FUNC_CASE("idelta", kIdelta)
    PROMQL_WINDOW_FUNC_CASE("min_over_time", kMinOverTime)
    PROMQL_WINDOW_FUNC_CASE("max_over_time", kMaxOverTime)
    PROMQL_WINDOW_FUNC_CASE("last_over_time", kLastOverTime)
    PROMQL_WINDOW_FUNC_CASE("sum_over_time", kSumOverTime)
    PROMQL_WINDOW_FUNC_CASE("delta", kDelta)
    PROMQL_WINDOW_FUNC_CASE("resets", kResets)
    PROMQL_WINDOW_FUNC_CASE("changes", kChanges)
    default:
      return WindowFunction::kNone;
  }

#undef PROMQL_WINDOW_FUNC_CASE

  return WindowFunction::kNone;
}

}  // namespace PromPP::Prometheus::promql