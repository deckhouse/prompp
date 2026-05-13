#pragma once

#include "primitives/primitives.h"
#include "series_data/decoder/universal_decode_iterator.h"

namespace series_data::decoder::decorator {

template <class Iterator>
concept WindowFunctionIteratorInterface = requires(Iterator iterator, const Iterator const_iterator) {
  { Iterator(PromPP::Primitives::TimeInterval()) };
  { Iterator(UniversalDecodeIterator(), PromPP::Primitives::TimeInterval()) };

  { const_iterator == DecodeIteratorSentinel() };
  { ++iterator } -> std::same_as<Iterator&>;
  { *iterator } -> std::same_as<const encoder::Sample&>;

  { const_iterator.interval() } -> std::same_as<const PromPP::Primitives::TimeInterval&>;

  { iterator.set_interval(PromPP::Primitives::TimeInterval()) };
  { iterator.reset(UniversalDecodeIterator(), PromPP::Primitives::TimeInterval()) };
};

struct WindowFunctionParameters {
  PromPP::Primitives::TimeInterval interval;
  PromPP::Primitives::Timestamp step;
  PromPP::Primitives::Timestamp range;
};

struct WindowBoundaryCalculator {
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
    if ((parameters.range % parameters.step) == 0) {
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

template <WindowFunctionIteratorInterface FunctionIterator>
class WindowFunctionIterator {
 public:
  using Timestamp = PromPP::Primitives::Timestamp;
  using TimeInterval = PromPP::Primitives::TimeInterval;

  DECODE_ITERATOR_TYPE_TRAITS();

  explicit WindowFunctionIterator(const WindowFunctionParameters& parameters) noexcept
      : iterator_{WindowBoundaryCalculator::initial_window(parameters)}, parameters_(&parameters) {
    advance();
  }

  explicit WindowFunctionIterator(UniversalDecodeIterator&& iterator, const WindowFunctionParameters& parameters)
      : iterator_(std::forward<UniversalDecodeIterator>(iterator), WindowBoundaryCalculator::initial_window(parameters)), parameters_(&parameters) {
    advance();
  }

  PROMPP_ALWAYS_INLINE WindowFunctionIterator& operator=(UniversalDecodeIterator&& iterator) {
    iterator_.reset(std::move(iterator), WindowBoundaryCalculator::initial_window(*parameters_));
    advance();
    return *this;
  }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const { return iterator_.operator*(); }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const { return iterator_.operator->(); }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel& sentinel) const noexcept { return iterator_ == sentinel; }

  PROMPP_ALWAYS_INLINE WindowFunctionIterator& operator++() {
    ++iterator_;
    advance();
    return *this;
  }

  PROMPP_ALWAYS_INLINE WindowFunctionIterator operator++(int) {
    const auto result = *this;
    ++*this;
    return result;
  }

 protected:
  FunctionIterator iterator_;
  const WindowFunctionParameters* parameters_;

  void advance() {
    while (iterator_ == DecodeIteratorSentinel{}) {
      if (const auto interval = WindowBoundaryCalculator::next_window(iterator_.interval(), *parameters_); interval.min < parameters_->interval.max)
          [[likely]] {
        iterator_.set_interval(interval);
      } else {
        break;
      }
    }
  }
};

}  // namespace series_data::decoder::decorator