#pragma once

#include "select_hints.h"
#include "series_data/decoder/decorator/last_over_step.h"
#include "series_data/decoder/decorator/lookback_delta_iterator.h"
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

  using LastOverTimeWithStaleNansIterator = ::series_data::decoder::decorator::LastOverTimeWithStaleNansIterator<SeriesIterator>;

  using Iterator = ::series_data::decoder::decorator::LookbackDeltaIterator<LastOverTimeWithStaleNansIterator>;

  using FindMinElement = ::series_data::decoder::decorator::FindMinElement;
  using FindMaxElement = ::series_data::decoder::decorator::FindMaxElement;
  using SumOfElements = ::series_data::decoder::decorator::SumOfElements;

  using MinMultiSeriesIterator = MultiSeriesIterator<Iterator, FindMinElement>;
  using MaxMultiSeriesIterator = MultiSeriesIterator<Iterator, FindMaxElement>;
  using SumMultiSeriesIterator = MultiSeriesIterator<Iterator, SumOfElements>;

  enum class Type : uint8_t {
    kMin = 0,
    kMax,
    kSum,
  };

  DECODE_ITERATOR_TYPE_TRAITS();

#define DEFINE_CONSTRUCTOR(MultiSeriesIteratorType, field, type)                                    \
  template <class... Args>                                                                          \
  explicit MultiSeriesDecodeIterator(std::in_place_type_t<MultiSeriesIteratorType>, Args&&... args) \
      : iterator_{.field{std::forward<Args>(args)...}}, type_{Type::type} {}

  DEFINE_CONSTRUCTOR(MinMultiSeriesIterator, min, kMin)
  DEFINE_CONSTRUCTOR(MaxMultiSeriesIterator, max, kMax)
  DEFINE_CONSTRUCTOR(SumMultiSeriesIterator, sum, kSum)

#undef DEFINE_CONSTRUCTOR

  template <class Visitor>
  PROMPP_ALWAYS_INLINE decltype(auto) visit(Visitor&& visitor) const {
    switch (type_) {
      case Type::kMin: {
        return std::forward<Visitor>(visitor)(iterator_.min);
      }

      case Type::kMax: {
        return std::forward<Visitor>(visitor)(iterator_.max);
      }

      default: {
        return std::forward<Visitor>(visitor)(iterator_.sum);
      }
    }
  }

  template <class Visitor>
  PROMPP_ALWAYS_INLINE decltype(auto) visit(Visitor&& visitor) {
    return const_cast<const MultiSeriesDecodeIterator*>(this)->visit(
        [&]<typename Iterator>(const Iterator& iterator) PROMPP_LAMBDA_INLINE { return std::forward<Visitor>(visitor)(const_cast<Iterator&>(iterator)); });
  }

  ~MultiSeriesDecodeIterator() {
    visit([](const auto& iterator) PROMPP_LAMBDA_INLINE { std::destroy_at(&iterator); });
  }

  PROMPP_ALWAYS_INLINE const ::series_data::encoder::Sample& operator*() const {
    return visit([](const auto& iterator) PROMPP_LAMBDA_INLINE -> const auto& { return *iterator; });
  }
  PROMPP_ALWAYS_INLINE const ::series_data::encoder::Sample* operator->() const {
    return visit([](const auto& iterator) PROMPP_LAMBDA_INLINE -> const auto* { return iterator.operator->(); });
  }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel& sentinel) const {
    return visit([&sentinel](const auto& iterator) PROMPP_LAMBDA_INLINE { return iterator == sentinel; });
  }

  PROMPP_ALWAYS_INLINE MultiSeriesDecodeIterator& operator++() {
    visit([]<typename Iterator>(Iterator& iterator) PROMPP_LAMBDA_INLINE { ++iterator; });
    return *this;
  }

  template <class IteratorsGenerator>
  PROMPP_ALWAYS_INLINE void reset(const ::series_data::decoder::decorator::WindowFunctionParameters& parameters, IteratorsGenerator&& iterators_generator) {
    visit([&](auto& iterator) PROMPP_LAMBDA_INLINE { iterator.reset(parameters, std::forward<IteratorsGenerator>(iterators_generator)); });
  }

  PROMPP_ALWAYS_INLINE static void create_series_iterators(const SelectHints& select_hints,
                                                           std::span<const uint32_t> series_ids,
                                                           ::series_data::serialization::SerializedDataView data_view,
                                                           BareBones::Vector<Iterator>& iterators) {
    const auto initial_interval = WindowBoundaryCalculator::initial_window(select_hints.function_parameters);

    iterators.reserve(series_ids.size());
    data_view.enumerate_series([&](const auto& chunk, uint32_t chunk_id) {
      if (std::ranges::find(series_ids, chunk.label_set_id) != series_ids.end()) {
        iterators.emplace_back(LastOverTimeWithStaleNansIterator(data_view.create_series_iterator(chunk_id), initial_interval),
                               select_hints.function_parameters.lookback_delta);
      }
    });
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Type type() const noexcept { return type_; }

 private:
  union Iterators {
    ~Iterators() {}

    MinMultiSeriesIterator min;
    MaxMultiSeriesIterator max;
    SumMultiSeriesIterator sum;
  } iterator_;

  Type type_;
};

PROMPP_ALWAYS_INLINE void construct_multi_series_decode_iterator(MultiSeriesDecodeIterator* iterator,
                                                                 const SelectHints& select_hints,
                                                                 std::span<const uint32_t> series_ids,
                                                                 ::series_data::serialization::SerializedDataView data_view) {
  const auto create_series_iterators = [&] {
    BareBones::Vector<MultiSeriesDecodeIterator::Iterator> iterators;
    MultiSeriesDecodeIterator::create_series_iterators(select_hints, series_ids, data_view, iterators);
    return iterators;
  };

  switch (select_hints.window_function) {
    using enum PromPP::Prometheus::promql::WindowFunction;

    case kMin: {
      std::construct_at(iterator, std::in_place_type<MultiSeriesDecodeIterator::MinMultiSeriesIterator>, create_series_iterators(),
                        select_hints.function_parameters);
      break;
    }

    case kMax: {
      std::construct_at(iterator, std::in_place_type<MultiSeriesDecodeIterator::MaxMultiSeriesIterator>, create_series_iterators(),
                        select_hints.function_parameters);
      break;
    }

    case kSum:
    default: {
      std::construct_at(iterator, std::in_place_type<MultiSeriesDecodeIterator::SumMultiSeriesIterator>, create_series_iterators(),
                        select_hints.function_parameters);
      break;
    }
  }
}

}  // namespace entrypoint::series_data
