#pragma once

#include <variant>

#include "select_hints.h"
#include "series_data/decoder/decorator/last_over_step.h"
#include "series_data/decoder/decorator/max_over_time.h"
#include "series_data/decoder/decorator/min_over_time.h"
#include "series_data/decoder/decorator/multiseries_iterator.h"
#include "series_data/decoder/decorator/sum_over_time.h"
#include "series_data/decoder/universal_decode_iterator.h"
#include "series_data/serialization/serialized_data.h"

namespace entrypoint::series_data {

class MultiSeriesDecodeIterator {
 public:
  using DecodeIteratorSentinel = ::series_data::decoder::DecodeIteratorSentinel;
  using UniversalDecodeIterator = ::series_data::decoder::UniversalDecodeIterator;
  using WindowBoundaryCalculator = ::series_data::decoder::decorator::StepLookbackDeltaWindowCalculator;

  template <class Iterator, class SampleHandler>
  using MultiSeriesIterator = ::series_data::decoder::decorator::MultiSeriesIterator<Iterator, SampleHandler, WindowBoundaryCalculator>;
  using SeriesIterator = ::series_data::serialization::SerializedDataView::SeriesIterator;

  using LastOverStepIterator = ::series_data::decoder::decorator::LastOverStepIterator<SeriesIterator>;

  using FindMinElement = ::series_data::decoder::decorator::FindMinElement;
  using FindMaxElement = ::series_data::decoder::decorator::FindMaxElement;
  using SumOfElements = ::series_data::decoder::decorator::SumOfElements;

  using MinMultiSeriesIterator = MultiSeriesIterator<LastOverStepIterator, FindMinElement>;
  using MaxMultiSeriesIterator = MultiSeriesIterator<LastOverStepIterator, FindMaxElement>;
  using SumMultiSeriesIterator = MultiSeriesIterator<LastOverStepIterator, SumOfElements>;

  using IteratorVariant = std::variant<MinMultiSeriesIterator, MaxMultiSeriesIterator, SumMultiSeriesIterator>;

  DECODE_ITERATOR_TYPE_TRAITS();

  template <class InPlaceType, class... Args>
  explicit MultiSeriesDecodeIterator(InPlaceType in_place_type, Args&&... args) : iterator_(in_place_type, std::forward<Args>(args)...) {}

  PROMPP_ALWAYS_INLINE const ::series_data::encoder::Sample& operator*() const {
    return std::visit([](auto& iterator) PROMPP_LAMBDA_INLINE -> auto const& { return *iterator; }, iterator_);
  }
  PROMPP_ALWAYS_INLINE const ::series_data::encoder::Sample* operator->() const {
    return std::visit([](auto& iterator) PROMPP_LAMBDA_INLINE -> auto const* { return iterator.operator->(); }, iterator_);
  }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel& sentinel) const {
    return std::visit([&sentinel](const auto& iterator) PROMPP_LAMBDA_INLINE { return iterator == sentinel; }, iterator_);
  }

  PROMPP_ALWAYS_INLINE MultiSeriesDecodeIterator& operator++() {
    std::visit([]<typename Iterator>(Iterator& iterator) PROMPP_LAMBDA_INLINE { ++iterator; }, iterator_);
    return *this;
  }

  PROMPP_ALWAYS_INLINE MultiSeriesDecodeIterator operator++(int) {
    const auto result = *this;
    ++*this;
    return result;
  }

 private:
  IteratorVariant iterator_;
};

PROMPP_ALWAYS_INLINE MultiSeriesDecodeIterator create_multi_series_decode_iterator(const SelectHints& select_hints,
                                                                                   std::span<const uint32_t> series_ids,
                                                                                   ::series_data::serialization::SerializedDataView data_view) {
  const auto create_series_iterators = [&] {
    BareBones::Vector<MultiSeriesDecodeIterator::LastOverStepIterator> iterators;
    iterators.reserve(series_ids.size());

    const auto initial_interval = MultiSeriesDecodeIterator::WindowBoundaryCalculator::initial_window(select_hints.function_parameters);

    data_view.enumerate_series([&](const auto& chunk, uint32_t chunk_id) {
      if (std::ranges::find(series_ids, chunk.label_set_id) != series_ids.end()) {
        iterators.emplace_back(data_view.create_series_iterator(chunk_id), initial_interval);
      }
    });

    return iterators;
  };

  switch (select_hints.window_function) {
    using enum PromPP::Prometheus::promql::WindowFunction;

    case kMin: {
      return MultiSeriesDecodeIterator(std::in_place_type<MultiSeriesDecodeIterator::MinMultiSeriesIterator>, create_series_iterators(),
                                       select_hints.function_parameters);
    }

    case kMax: {
      return MultiSeriesDecodeIterator(std::in_place_type<MultiSeriesDecodeIterator::MaxMultiSeriesIterator>, create_series_iterators(),
                                       select_hints.function_parameters);
    }

    case kSum:
    default: {
      return MultiSeriesDecodeIterator(std::in_place_type<MultiSeriesDecodeIterator::SumMultiSeriesIterator>, create_series_iterators(),
                                       select_hints.function_parameters);
    }
  }
}

}  // namespace entrypoint::series_data