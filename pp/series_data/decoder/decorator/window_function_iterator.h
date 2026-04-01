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

template <WindowFunctionIteratorInterface FunctionIterator>
class WindowFunctionIterator {
 public:
  using Timestamp = PromPP::Primitives::Timestamp;
  using TimeInterval = PromPP::Primitives::TimeInterval;

  DECODE_ITERATOR_TYPE_TRAITS();

  explicit WindowFunctionIterator(const TimeInterval& interval, Timestamp step_ms, Timestamp range_ms)
      : iterator_{initial_interval(interval, step_ms, range_ms)}, full_interval_(interval), step_ms_(step_ms), range_ms_(range_ms) {
    advance();
  }

  explicit WindowFunctionIterator(UniversalDecodeIterator&& iterator, const TimeInterval& interval, Timestamp step_ms, Timestamp range_ms)
      : iterator_(std::forward<UniversalDecodeIterator>(iterator), initial_interval(interval, step_ms, range_ms)),
        full_interval_(interval),
        step_ms_(step_ms),
        range_ms_(range_ms) {
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
  TimeInterval full_interval_;
  int64_t step_ms_;
  int64_t range_ms_;

  void advance() {
    while (iterator_ == DecodeIteratorSentinel{}) {
      iterator_.set_interval(advance_interval());
      if (interval_is_exceeded()) [[unlikely]] {
        break;
      }
    };
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool interval_is_exceeded() const noexcept { return iterator_.interval().min >= full_interval_.max; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE TimeInterval advance_interval() const noexcept {
    auto interval = iterator_.interval();
    if (interval.difference() == step_ms_) {
      interval.min = interval.max;
      interval.max += (range_ms_ == step_ms_) ? step_ms_ : (range_ms_ - step_ms_);
    } else {
      interval.min = std::exchange(interval.max, next_interval_boundary(interval.min));
    }

    interval.max = std::min(interval.max, full_interval_.max);
    return interval;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE TimeInterval initial_interval() const noexcept { return initial_interval(full_interval_, step_ms_, range_ms_); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE static TimeInterval initial_interval(const TimeInterval& full_interval, Timestamp step_ms, Timestamp range_ms) noexcept {
    if (step_ms == 0) {
      return full_interval;
    }

    return {.min = full_interval.min, .max = std::min(next_interval_boundary(full_interval.min, step_ms, range_ms), full_interval.max)};
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Timestamp next_interval_boundary(Timestamp start) const noexcept {
    return next_interval_boundary(start, step_ms_, range_ms_);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static Timestamp next_interval_boundary(Timestamp start, Timestamp step_ms, Timestamp range_ms) noexcept {
    if (range_ms <= step_ms) [[likely]] {
      return start + range_ms;
    }

    return start + range_ms - (range_ms - step_ms);
  }
};

}  // namespace series_data::decoder::decorator