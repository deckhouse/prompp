#pragma once

#include "bare_bones/gorilla.h"
#include "series_data/decoder/traits.h"
#include "window_boundary_calculator.h"

namespace series_data::decoder::decorator {

template <class Iterator, class SampleHandler, WindowBoundaryCalculatorInterface WindowBoundaryCalculator>
class MultiSeriesIterator {
 public:
  DECODE_ITERATOR_TYPE_TRAITS();

  explicit MultiSeriesIterator(BareBones::Vector<Iterator>&& iterators, const WindowFunctionParameters& parameters)
      : iterators_(std::move(iterators)), parameters_(&parameters) {
    seek_to_first_non_stale_nan_sample();
  }

  PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const { return sample_; }
  PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const { return &sample_; }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const { return sample_.timestamp == kInvalidTimestamp; }

  PROMPP_ALWAYS_INLINE MultiSeriesIterator& operator++() {
    update_sample();
    return *this;
  }

  PROMPP_ALWAYS_INLINE MultiSeriesIterator operator++(int) {
    const auto result = *this;
    ++*this;
    return result;
  }

  template <class IteratorsGenerator>
  PROMPP_ALWAYS_INLINE void reset(const WindowFunctionParameters& parameters, IteratorsGenerator&& iterators_generator) {
    iterators_.clear();
    std::forward<IteratorsGenerator>(iterators_generator)(iterators_);

    parameters_ = &parameters;

    seek_to_first_non_stale_nan_sample();
  }

 private:
  encoder::Sample sample_;
  BareBones::Vector<Iterator> iterators_;
  const WindowFunctionParameters* parameters_;

  PROMPP_ALWAYS_INLINE void seek_to_first_non_stale_nan_sample() {
    do {
      update_sample();
    } while (*this != DecodeIteratorSentinel{} && BareBones::Encoding::Gorilla::isstalenan(sample_.value));
  }

  void update_sample() {
    sample_ = encoder::Sample{.timestamp = kInvalidTimestamp, .value = BareBones::Encoding::Gorilla::STALE_NAN};

    if (iterators_.empty()) [[unlikely]] {
      return;
    }

    const auto current_window = iterators_[0].interval();
    if (current_window.min > current_window.max) [[unlikely]] {
      return;
    }

    handle_samples(current_window, WindowBoundaryCalculator::next_window(current_window, *parameters_));
    sample_.timestamp = current_window.max;
  }

  PROMPP_ALWAYS_INLINE void handle_samples(const PromPP::Primitives::TimeInterval& current_window,
                                           const PromPP::Primitives::TimeInterval& next_window) noexcept {
    SampleHandler handler{sample_, current_window};
    for (auto it = iterators_.begin(); it != iterators_.end();) {
      auto& iterator = *it;
      if (iterator != DecodeIteratorSentinel{}) [[likely]] {
        handler(iterator->timestamp, iterator->value);
      } else if (!iterator.has_more_samples()) {
        iterators_.erase(it);
        continue;
      }

      iterator.set_interval(next_window);
      ++it;
    }
  }
};

}  // namespace series_data::decoder::decorator
