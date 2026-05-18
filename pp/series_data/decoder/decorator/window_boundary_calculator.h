#pragma once

#include "primitives/primitives.h"

namespace series_data::decoder::decorator {

struct WindowFunctionParameters {
  PromPP::Primitives::TimeInterval interval;
  PromPP::Primitives::Timestamp step;
  PromPP::Primitives::Timestamp range;
};

template <class WindowCalculator>
concept WindowBoundaryCalculatorInterface =
    requires(WindowCalculator calculator, const PromPP::Primitives::TimeInterval& current_window, const WindowFunctionParameters& parameters) {
      { WindowCalculator::initial_window(parameters) } -> std::same_as<PromPP::Primitives::TimeInterval>;
      { WindowCalculator::next_window(current_window, parameters) } -> std::same_as<PromPP::Primitives::TimeInterval>;
    };

struct StepRangeWindowCalculator {
  using Timestamp = PromPP::Primitives::Timestamp;
  using TimeInterval = PromPP::Primitives::TimeInterval;

  [[nodiscard]] PROMPP_ALWAYS_INLINE static TimeInterval initial_window(const WindowFunctionParameters& parameters) noexcept {
    TimeInterval result{.min = parameters.interval.min + 1};

    if (parameters.step == 0) {
      result.max = parameters.interval.max;
    } else if (has_gap_between_windows(parameters)) {
      result.max = std::min(parameters.interval.min + parameters.range, parameters.interval.max);
    } else {
      result.max = std::min(next_interval_boundary(parameters.interval.min, parameters), parameters.interval.max);
    }

    return result;
  }

  [[nodiscard]] static PROMPP_ALWAYS_INLINE TimeInterval next_window(const TimeInterval& current_window, const WindowFunctionParameters& parameters) noexcept {
    TimeInterval interval{};
    if (has_gap_between_windows(parameters)) {
      interval.min = current_window.min + parameters.step;
      interval.max = interval.min + parameters.range - 1;
    } else {
      interval.min = current_window.max + 1;
      interval.max = next_interval_boundary(current_window.max, parameters);
    }

    interval.max = std::min(interval.max, parameters.interval.max);
    return interval;
  }

 private:
  [[nodiscard]] PROMPP_ALWAYS_INLINE static Timestamp next_interval_boundary(Timestamp start, const WindowFunctionParameters& parameters) noexcept {
    if (parameters.step != 0 && (parameters.range % parameters.step) == 0) {
      return start + parameters.step;
    }

    if (parameters.step == 0 || parameters.range < parameters.step) {
      return start + parameters.range;
    }

    const auto eval_start = parameters.interval.min + parameters.range;
    const auto remainder = parameters.range % parameters.step;
    return std::min(next_aligned_boundary(start, eval_start, parameters.step), next_aligned_boundary(start, eval_start - remainder, parameters.step));
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static bool has_gap_between_windows(const WindowFunctionParameters& parameters) noexcept {
    return parameters.range > 0 && parameters.range < parameters.step;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static Timestamp next_aligned_boundary(Timestamp start, Timestamp anchor, Timestamp step) noexcept {
    const auto quotient = floor_div(start - anchor, step) + 1;
    return anchor + quotient * step;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static Timestamp floor_div(Timestamp numerator, Timestamp denominator) noexcept {
    auto quotient = numerator / denominator;
    if (numerator % denominator < 0) {
      --quotient;
    }

    return quotient;
  }
};

static_assert(WindowBoundaryCalculatorInterface<StepRangeWindowCalculator>);

}  // namespace series_data::decoder::decorator