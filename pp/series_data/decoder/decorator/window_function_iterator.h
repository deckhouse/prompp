#pragma once

#include "primitives/primitives.h"
#include "series_data/decoder/universal_decode_iterator.h"
#include "window_boundary_calculator.h"

namespace series_data::decoder::decorator {

template <class Iterator>
concept WindowFunctionIteratorInterface = requires(Iterator iterator, const Iterator const_iterator) {
  { Iterator(PromPP::Primitives::TimeInterval()) };

  { const_iterator == DecodeIteratorSentinel() };
  { ++iterator } -> std::same_as<Iterator&>;
  { *iterator } -> std::same_as<const encoder::Sample&>;

  { const_iterator.interval() } -> std::same_as<const PromPP::Primitives::TimeInterval&>;

  { iterator.set_interval(PromPP::Primitives::TimeInterval()) };
};

template <WindowFunctionIteratorInterface FunctionIterator, WindowBoundaryCalculatorInterface WindowBoundaryCalculator>
class WindowFunctionIterator {
 public:
  using Timestamp = PromPP::Primitives::Timestamp;
  using TimeInterval = PromPP::Primitives::TimeInterval;

  DECODE_ITERATOR_TYPE_TRAITS();

  explicit WindowFunctionIterator(const WindowFunctionParameters& parameters) noexcept
      : iterator_{WindowBoundaryCalculator::initial_window(parameters)}, parameters_(&parameters) {
    advance();
  }

  template <class Iterator>
  explicit WindowFunctionIterator(Iterator&& iterator, const WindowFunctionParameters& parameters)
      : iterator_(std::forward<Iterator>(iterator), WindowBoundaryCalculator::initial_window(parameters)), parameters_(&parameters) {
    advance();
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
