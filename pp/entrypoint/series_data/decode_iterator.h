#pragma once

#include <variant>

#include "prometheus/promql/function_names_hash.h"
#include "prometheus/query.h"
#include "series_data/decoder/decorator/changes_iterator.h"
#include "series_data/decoder/decorator/delta_iterator.h"
#include "series_data/decoder/decorator/downsampling_decode_iterator.h"
#include "series_data/decoder/decorator/irate_iterator.h"
#include "series_data/decoder/decorator/last_over_time.h"
#include "series_data/decoder/decorator/max_over_time.h"
#include "series_data/decoder/decorator/min_over_time.h"
#include "series_data/decoder/decorator/rate_iterator.h"
#include "series_data/decoder/decorator/resets_iterator.h"
#include "series_data/decoder/decorator/sum_over_time.h"
#include "series_data/decoder/decorator/window_function_iterator.h"
#include "series_data/decoder/universal_decode_iterator.h"

namespace entrypoint::series_data {

template <class Iterator>
concept invalidatable = requires(Iterator iterator) {
  { iterator.invalidate_sample() };
};

class DecodeIterator {
 public:
  using DecodeIteratorSentinel = ::series_data::decoder::DecodeIteratorSentinel;
  using UniversalDecodeIterator = ::series_data::decoder::UniversalDecodeIterator;
  using DownsamplingIterator = ::series_data::decoder::decorator::DownsamplingDecodeIterator<UniversalDecodeIterator>;

  template <class Iterator>
  using WindowFunctionIterator = ::series_data::decoder::decorator::WindowFunctionIterator<Iterator>;
  using MinOverTimeIterator = WindowFunctionIterator<::series_data::decoder::decorator::MinOverTimeIterator>;
  using MaxOverTimeIterator = WindowFunctionIterator<::series_data::decoder::decorator::MaxOverTimeIterator>;
  using LastOverTimeIterator = WindowFunctionIterator<::series_data::decoder::decorator::LastOverTimeIterator>;
  using SumOverTimeIterator = WindowFunctionIterator<::series_data::decoder::decorator::SumOverTimeIterator>;
  using RateIterator = WindowFunctionIterator<::series_data::decoder::decorator::RateIterator>;
  using IRateIterator = WindowFunctionIterator<::series_data::decoder::decorator::IRateIterator>;
  using ChangesIterator = WindowFunctionIterator<::series_data::decoder::decorator::ChangesIterator>;
  using DeltaIterator = WindowFunctionIterator<::series_data::decoder::decorator::DeltaIterator>;
  using ResetsIterator = WindowFunctionIterator<::series_data::decoder::decorator::ResetsIterator>;

  using IteratorVariant = std::variant<UniversalDecodeIterator,
                                       DownsamplingIterator,
                                       MinOverTimeIterator,
                                       MaxOverTimeIterator,
                                       LastOverTimeIterator,
                                       SumOverTimeIterator,
                                       RateIterator,
                                       IRateIterator,
                                       ChangesIterator,
                                       DeltaIterator,
                                       ResetsIterator>;

  DECODE_ITERATOR_TYPE_TRAITS();

  template <class InPlaceType, class... Args>
  explicit DecodeIterator(InPlaceType in_place_type, Args&&... args) : iterator_(in_place_type, std::forward<Args>(args)...) {}

  PROMPP_ALWAYS_INLINE DecodeIterator& operator=(UniversalDecodeIterator&& it) {
    std::visit([&it](auto& iterator) PROMPP_LAMBDA_INLINE { iterator = std::move(it); }, iterator_);
    return *this;
  }

  PROMPP_ALWAYS_INLINE const ::series_data::encoder::Sample& operator*() const {
    return std::visit([](auto& iterator) PROMPP_LAMBDA_INLINE -> auto const& { return *iterator; }, iterator_);
  }
  PROMPP_ALWAYS_INLINE const ::series_data::encoder::Sample* operator->() const {
    return std::visit([](auto& iterator) PROMPP_LAMBDA_INLINE -> auto const* { return iterator.operator->(); }, iterator_);
  }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel& sentinel) const {
    return std::visit([&sentinel](const auto& iterator) PROMPP_LAMBDA_INLINE { return iterator == sentinel; }, iterator_);
  }

  PROMPP_ALWAYS_INLINE DecodeIterator& operator++() {
    std::visit(
        []<typename Iterator>(Iterator& iterator) PROMPP_LAMBDA_INLINE {
          ++iterator;

          if constexpr (invalidatable<Iterator>) {
            if (iterator == DecodeIteratorSentinel{}) [[unlikely]] {
              iterator.invalidate_sample();
            }
          }
        },
        iterator_);
    return *this;
  }

  PROMPP_ALWAYS_INLINE DecodeIterator operator++(int) {
    const auto result = *this;
    ++*this;
    return result;
  }

 private:
  IteratorVariant iterator_;
};

struct SelectHints {
  std::string func;
  ::series_data::decoder::decorator::WindowFunctionParameters function_parameters;
};

constexpr uint32_t promql_function_name_hash(std::string_view str) {
  return PromPP::Prometheus::promql::FunctionNamesHash::hash(str.data(), str.length());
}

PROMPP_ALWAYS_INLINE DecodeIterator create_decode_iterator(const SelectHints& select_hints, PromPP::Primitives::Timestamp downsampling_ms) {
  if (downsampling_ms != ::series_data::decoder::decorator::kNoDownsampling) [[unlikely]] {
    return DecodeIterator(std::in_place_type<DecodeIterator::DownsamplingIterator>, downsampling_ms);
  }
  if (select_hints.func.empty()) [[likely]] {
    return DecodeIterator(std::in_place_type<DecodeIterator::UniversalDecodeIterator>);
  }

#define CASE(function_name, iterator_type)                                                      \
  case promql_function_name_hash(function_name): {                                              \
    if (select_hints.func != function_name) [[unlikely]] {                                      \
      break;                                                                                    \
    }                                                                                           \
    return DecodeIterator(std::in_place_type<iterator_type>, select_hints.function_parameters); \
  }

  switch (promql_function_name_hash(select_hints.func)) {
    CASE("rate", DecodeIterator::RateIterator)
    CASE("increase", DecodeIterator::RateIterator)

    CASE("irate", DecodeIterator::IRateIterator)
    CASE("idelta", DecodeIterator::IRateIterator)

    CASE("min_over_time", DecodeIterator::MinOverTimeIterator)
    CASE("max_over_time", DecodeIterator::MaxOverTimeIterator)
    CASE("last_over_time", DecodeIterator::LastOverTimeIterator)
    CASE("sum_over_time", DecodeIterator::SumOverTimeIterator)

    CASE("delta", DecodeIterator::DeltaIterator)

    CASE("resets", DecodeIterator::ResetsIterator)

    CASE("changes", DecodeIterator::ChangesIterator)

    default:;
  }

#undef CASE

  return DecodeIterator(std::in_place_type<DecodeIterator::UniversalDecodeIterator>);
}

}  // namespace entrypoint::series_data