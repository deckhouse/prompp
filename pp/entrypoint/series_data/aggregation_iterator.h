#pragma once

#include "prometheus/promql/window_function.h"
#include "select_hints.h"
#include "series_data/decoder/decorator/changes_iterator.h"
#include "series_data/decoder/decorator/delta_iterator.h"
#include "series_data/decoder/decorator/downsampling_decode_iterator.h"
#include "series_data/decoder/decorator/irate_iterator.h"
#include "series_data/decoder/decorator/last_over_step.h"
#include "series_data/decoder/decorator/last_over_time.h"
#include "series_data/decoder/decorator/max_over_time.h"
#include "series_data/decoder/decorator/min_over_time.h"
#include "series_data/decoder/decorator/rate_iterator.h"
#include "series_data/decoder/decorator/resets_iterator.h"
#include "series_data/decoder/decorator/sum_over_time.h"
#include "series_data/decoder/decorator/window_function_iterator.h"
#include "series_data/decoder/universal_decode_iterator.h"
#include "series_data/serialization/serialized_data.h"

namespace entrypoint::series_data {

template <class Iterator>
concept invalidatable = requires(Iterator iterator) {
  { iterator.invalidate_sample() };
};

class AggregationIterator {
 public:
  using DecodeIteratorSentinel = ::series_data::decoder::DecodeIteratorSentinel;
  using UniversalDecodeIterator = ::series_data::decoder::UniversalDecodeIterator;
  using SeriesIterator = ::series_data::serialization::SerializedDataView::SeriesIterator;
  using DownsamplingIterator = ::series_data::decoder::decorator::DownsamplingDecodeIterator<SeriesIterator>;

  template <class Iterator,
            ::series_data::decoder::decorator::WindowBoundaryCalculatorInterface WindowBoundaryCalculator =
                ::series_data::decoder::decorator::StepRangeWindowCalculator>
  using WindowFunctionIterator = ::series_data::decoder::decorator::WindowFunctionIterator<Iterator, WindowBoundaryCalculator>;
  using MinOverTimeIterator = WindowFunctionIterator<::series_data::decoder::decorator::MinOverTimeIterator<SeriesIterator>>;
  using MaxOverTimeIterator = WindowFunctionIterator<::series_data::decoder::decorator::MaxOverTimeIterator<SeriesIterator>>;
  using LastOverTimeIterator = WindowFunctionIterator<::series_data::decoder::decorator::LastOverTimeIterator<SeriesIterator>>;
  using LastOverStepIterator = WindowFunctionIterator<::series_data::decoder::decorator::LastOverStepIterator<SeriesIterator>,
                                                      ::series_data::decoder::decorator::StepLookbackDeltaWindowCalculator>;
  using SumOverTimeIterator = WindowFunctionIterator<::series_data::decoder::decorator::SumOverTimeIterator<SeriesIterator>>;
  using RateIterator = WindowFunctionIterator<::series_data::decoder::decorator::RateIterator<SeriesIterator>>;
  using IRateIterator = WindowFunctionIterator<::series_data::decoder::decorator::IRateIterator<SeriesIterator>>;
  using ChangesIterator = WindowFunctionIterator<::series_data::decoder::decorator::ChangesIterator<SeriesIterator>>;
  using DeltaIterator = WindowFunctionIterator<::series_data::decoder::decorator::DeltaIterator<SeriesIterator>>;
  using ResetsIterator = WindowFunctionIterator<::series_data::decoder::decorator::ResetsIterator<SeriesIterator>>;

  enum class Type : uint8_t {
    kSeries = 0,
    kDownsampling,
    kMinOverTime,
    kMaxOverTime,
    kLastOverTime,
    kLastOverStep,
    kSumOverTime,
    kRate,
    kIRate,
    kChanges,
    kDelta,
    kResets,
  };

  DECODE_ITERATOR_TYPE_TRAITS();

#define DEFINE_CONSTRUCTOR(AggregationIteratorType, field, type)                              \
  template <class... Args>                                                                    \
  explicit AggregationIterator(std::in_place_type_t<AggregationIteratorType>, Args&&... args) \
      : iterator_{.field{std::forward<Args>(args)...}}, type_{Type::type} {}

  DEFINE_CONSTRUCTOR(SeriesIterator, series, kSeries)
  DEFINE_CONSTRUCTOR(DownsamplingIterator, downsampling, kDownsampling)
  DEFINE_CONSTRUCTOR(MinOverTimeIterator, min_over_time, kMinOverTime)
  DEFINE_CONSTRUCTOR(MaxOverTimeIterator, max_over_time, kMaxOverTime)
  DEFINE_CONSTRUCTOR(LastOverTimeIterator, last_over_time, kLastOverTime)
  DEFINE_CONSTRUCTOR(LastOverStepIterator, last_over_step, kLastOverStep)
  DEFINE_CONSTRUCTOR(SumOverTimeIterator, sum_over_time, kSumOverTime)
  DEFINE_CONSTRUCTOR(RateIterator, rate, kRate)
  DEFINE_CONSTRUCTOR(IRateIterator, irate, kIRate)
  DEFINE_CONSTRUCTOR(ChangesIterator, changes, kChanges)
  DEFINE_CONSTRUCTOR(DeltaIterator, delta, kDelta)
  DEFINE_CONSTRUCTOR(ResetsIterator, resets, kResets)

#undef DEFINE_CONSTRUCTOR

  template <class Visitor>
  PROMPP_ALWAYS_INLINE decltype(auto) visit(Visitor&& visitor) const {
    switch (type_) {
      case Type::kSeries: {
        return std::forward<Visitor>(visitor)(iterator_.series);
      }

      case Type::kDownsampling: {
        return std::forward<Visitor>(visitor)(iterator_.downsampling);
      }

      case Type::kMinOverTime: {
        return std::forward<Visitor>(visitor)(iterator_.min_over_time);
      }

      case Type::kMaxOverTime: {
        return std::forward<Visitor>(visitor)(iterator_.max_over_time);
      }

      case Type::kLastOverTime: {
        return std::forward<Visitor>(visitor)(iterator_.last_over_time);
      }

      case Type::kLastOverStep: {
        return std::forward<Visitor>(visitor)(iterator_.last_over_step);
      }

      case Type::kSumOverTime: {
        return std::forward<Visitor>(visitor)(iterator_.sum_over_time);
      }

      case Type::kRate: {
        return std::forward<Visitor>(visitor)(iterator_.rate);
      }

      case Type::kIRate: {
        return std::forward<Visitor>(visitor)(iterator_.irate);
      }

      case Type::kChanges: {
        return std::forward<Visitor>(visitor)(iterator_.changes);
      }

      case Type::kDelta: {
        return std::forward<Visitor>(visitor)(iterator_.delta);
      }

      default: {
        return std::forward<Visitor>(visitor)(iterator_.resets);
      }
    }
  }

  template <class Visitor>
  PROMPP_ALWAYS_INLINE decltype(auto) visit(Visitor&& visitor) {
    return const_cast<const AggregationIterator*>(this)->visit(
        [&]<typename Iterator>(const Iterator& iterator) PROMPP_LAMBDA_INLINE { return std::forward<Visitor>(visitor)(const_cast<Iterator&>(iterator)); });
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

  PROMPP_ALWAYS_INLINE AggregationIterator& operator++() {
    visit([]<typename Iterator>(Iterator& iterator) PROMPP_LAMBDA_INLINE {
      ++iterator;

      if constexpr (invalidatable<Iterator>) {
        if (iterator == DecodeIteratorSentinel{}) [[unlikely]] {
          iterator.invalidate_sample();
        }
      }
    });
    return *this;
  }

  PROMPP_ALWAYS_INLINE AggregationIterator operator++(int) {
    const auto result = *this;
    ++*this;
    return result;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE Type type() const noexcept { return type_; }

 private:
  union {
    SeriesIterator series;
    DownsamplingIterator downsampling;
    MinOverTimeIterator min_over_time;
    MaxOverTimeIterator max_over_time;
    LastOverTimeIterator last_over_time;
    LastOverStepIterator last_over_step;
    SumOverTimeIterator sum_over_time;
    RateIterator rate;
    IRateIterator irate;
    ChangesIterator changes;
    DeltaIterator delta;
    ResetsIterator resets;
  } iterator_;

  Type type_;
};

PROMPP_ALWAYS_INLINE AggregationIterator create_aggregation_iterator(::series_data::serialization::SerializedDataView::SeriesIterator&& iterator,
                                                                     const SelectHints& select_hints,
                                                                     PromPP::Primitives::Timestamp downsampling_ms) {
  if (downsampling_ms != ::series_data::decoder::decorator::kNoDownsampling) [[unlikely]] {
    return AggregationIterator(std::in_place_type<AggregationIterator::DownsamplingIterator>, std::move(iterator), downsampling_ms);
  }

  using enum PromPP::Prometheus::promql::WindowFunction;

  switch (select_hints.window_function) {
    case kRate:
    case kIncrease:
      return AggregationIterator(std::in_place_type<AggregationIterator::RateIterator>, std::move(iterator), select_hints.function_parameters);

    case kIrate:
    case kIdelta:
      return AggregationIterator(std::in_place_type<AggregationIterator::IRateIterator>, std::move(iterator), select_hints.function_parameters);

    case kMinOverTime:
      return AggregationIterator(std::in_place_type<AggregationIterator::MinOverTimeIterator>, std::move(iterator), select_hints.function_parameters);

    case kMaxOverTime:
      return AggregationIterator(std::in_place_type<AggregationIterator::MaxOverTimeIterator>, std::move(iterator), select_hints.function_parameters);

    case kLastOverTime:
      return AggregationIterator(std::in_place_type<AggregationIterator::LastOverTimeIterator>, std::move(iterator), select_hints.function_parameters);

    case kLastOverStep:
      return AggregationIterator(std::in_place_type<AggregationIterator::LastOverStepIterator>, std::move(iterator), select_hints.function_parameters);

    case kSumOverTime:
      return AggregationIterator(std::in_place_type<AggregationIterator::SumOverTimeIterator>, std::move(iterator), select_hints.function_parameters);

    case kDelta:
      return AggregationIterator(std::in_place_type<AggregationIterator::DeltaIterator>, std::move(iterator), select_hints.function_parameters);

    case kResets:
      return AggregationIterator(std::in_place_type<AggregationIterator::ResetsIterator>, std::move(iterator), select_hints.function_parameters);

    case kChanges:
      return AggregationIterator(std::in_place_type<AggregationIterator::ChangesIterator>, std::move(iterator), select_hints.function_parameters);

    default:
      return AggregationIterator(std::in_place_type<AggregationIterator::SeriesIterator>, std::move(iterator));
  }
}

}  // namespace entrypoint::series_data
