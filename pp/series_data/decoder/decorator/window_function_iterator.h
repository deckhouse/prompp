#pragma once

#include "primitives/primitives.h"
#include "series_data/decoder/universal_decode_iterator.h"

namespace series_data::decoder::decorator {

template <class Iterator>
concept WindowFunctionIteratorInterface = requires(Iterator iterator, const Iterator const_iterator) {
  { const_iterator.interval() } -> std::same_as<const PromPP::Primitives::TimeInterval&>;

  { iterator.set_interval(PromPP::Primitives::TimeInterval()) };
  { iterator.reset(UniversalDecodeIterator(), PromPP::Primitives::TimeInterval()) };
};

struct WindowFunctionParameters {
  PromPP::Primitives::TimeInterval interval;
  PromPP::Primitives::Timestamp step;
  PromPP::Primitives::Timestamp range;
};

template <WindowFunctionIteratorInterface FunctionIterator>
class WindowFunctionIterator {
 public:
  using Timestamp = PromPP::Primitives::Timestamp;
  using TimeInterval = PromPP::Primitives::TimeInterval;

  DECODE_ITERATOR_TYPE_TRAITS();

  explicit WindowFunctionIterator(const WindowFunctionParameters& parameters) noexcept : iterator_{initial_interval(parameters)}, parameters_(&parameters) {
    advance();
  }

  explicit WindowFunctionIterator(UniversalDecodeIterator&& iterator, const WindowFunctionParameters& parameters)
      : iterator_(std::forward<UniversalDecodeIterator>(iterator), initial_interval(parameters)), parameters_(&parameters) {
    advance();
  }

  PROMPP_ALWAYS_INLINE WindowFunctionIterator& operator=(UniversalDecodeIterator&& iterator) {
    iterator_.reset(std::move(iterator), initial_interval());
    advance();
    return *this;
  }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const { return iterator_.operator*(); }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const { return iterator_.operator->(); }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel& sentinel) const noexcept { return interval_is_exceeded() && iterator_ == sentinel; }

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
      iterator_.set_interval(advance_interval());
      if (interval_is_exceeded()) [[unlikely]] {
        break;
      }
    };
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool interval_is_exceeded() const noexcept { return iterator_.interval().min >= parameters_->interval.max; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE TimeInterval advance_interval() const noexcept {
    auto interval = iterator_.interval();
    if (interval.difference() == parameters_->step) {
      interval.min = interval.max;

      const auto diff = parameters_->range - parameters_->step;
      interval.max += (diff == 0) ? parameters_->step : diff;
    } else {
      interval.min = std::exchange(interval.max, next_interval_boundary(interval.min));
    }

    interval.max = std::min(interval.max, parameters_->interval.max);
    return interval;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE TimeInterval initial_interval() const noexcept { return initial_interval(*parameters_); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE static TimeInterval initial_interval(const WindowFunctionParameters& parameters) noexcept {
    if (parameters.step == 0) {
      return parameters.interval;
    }

    return {
        .min = parameters.interval.min,
        .max = std::min(next_interval_boundary(parameters.interval.min, parameters.step, parameters.range), parameters.interval.max),
    };
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Timestamp next_interval_boundary(Timestamp start) const noexcept {
    return next_interval_boundary(start, parameters_->step, parameters_->range);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static Timestamp next_interval_boundary(Timestamp start, Timestamp step_ms, Timestamp range_ms) noexcept {
    if (range_ms <= step_ms) [[likely]] {
      return start + range_ms;
    }

    return start + range_ms - (range_ms - step_ms);
  }
};

}  // namespace series_data::decoder::decorator