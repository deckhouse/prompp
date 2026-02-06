#pragma once

#include <variant>

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
#include "series_data/decoder/universal_decode_iterator.h"

namespace entrypoint::series_data {

template <class Iterator>
concept invalidatable = requires(Iterator iterator) {
  { iterator.invalidate() };
};

class DecodeIterator {
 public:
  using UniversalDecodeIterator = ::series_data::decoder::UniversalDecodeIterator;
  using DownsamplingIterator = ::series_data::decoder::decorator::DownsamplingDecodeIterator<UniversalDecodeIterator>;
  using MinOverTimeIterator = ::series_data::decoder::decorator::MinOverTimeIterator;
  using MaxOverTimeIterator = ::series_data::decoder::decorator::MaxOverTimeIterator;
  using LastOverTimeIterator = ::series_data::decoder::decorator::LastOverTimeIterator;
  using SumOverTimeIterator = ::series_data::decoder::decorator::SumOverTimeIterator;
  using RateIterator = ::series_data::decoder::decorator::RateIterator;
  using IRateIterator = ::series_data::decoder::decorator::IRateIterator;
  using ChangesIterator = ::series_data::decoder::decorator::ChangesIterator;
  using DeltaIterator = ::series_data::decoder::decorator::DeltaIterator;
  using ResetsIterator = ::series_data::decoder::decorator::ResetsIterator;
  using DecodeIteratorSentinel = ::series_data::decoder::DecodeIteratorSentinel;

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
              iterator.invalidate();
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

constexpr std::uint32_t promql_function_name_hash(std::string_view str) {
  constexpr std::uint32_t BASIS = 0xDEADBEEF;

  auto hash{BASIS};
  for (const auto c : str.substr(0)) {
    hash ^= static_cast<std::uint32_t>(c);
    hash <<= 1;
  }

  return hash;
}

PROMPP_ALWAYS_INLINE DecodeIterator create_decode_iterator(const PromPP::Prometheus::SelectHints& select_hints, PromPP::Primitives::Timestamp downsampling_ms) {
  if (downsampling_ms != ::series_data::decoder::decorator::kNoDownsampling) [[unlikely]] {
    return DecodeIterator(std::in_place_type<DecodeIterator::DownsamplingIterator>, downsampling_ms);
  }

  switch (promql_function_name_hash(select_hints.func)) {
    case promql_function_name_hash("rate"):
    case promql_function_name_hash("increase"):
      return DecodeIterator(std::in_place_type<DecodeIterator::RateIterator>, select_hints.interval);

    case promql_function_name_hash("irate"):
    case promql_function_name_hash("idelta"):
      return DecodeIterator(std::in_place_type<DecodeIterator::IRateIterator>, select_hints.interval);

    case promql_function_name_hash("min_over_time"):
      return DecodeIterator(std::in_place_type<DecodeIterator::MinOverTimeIterator>, select_hints.interval);

    case promql_function_name_hash("max_over_time"):
      return DecodeIterator(std::in_place_type<DecodeIterator::MaxOverTimeIterator>, select_hints.interval);

    case promql_function_name_hash("last_over_time"):
      return DecodeIterator(std::in_place_type<DecodeIterator::LastOverTimeIterator>, select_hints.interval);

    case promql_function_name_hash("sum_over_time"):
      return DecodeIterator(std::in_place_type<DecodeIterator::SumOverTimeIterator>, select_hints.interval);

    case promql_function_name_hash("delta"):
      return DecodeIterator(std::in_place_type<DecodeIterator::DeltaIterator>, select_hints.interval);

    case promql_function_name_hash("resets"):
      return DecodeIterator(std::in_place_type<DecodeIterator::ResetsIterator>, select_hints.interval);

      // A list of possible functions is needed to compile-time protect against collisions in the hash function.
      // The list of all functions specified in the promql/functions.go
    case promql_function_name_hash("abs"):
    case promql_function_name_hash("absent"):
    case promql_function_name_hash("absent_over_time"):
    case promql_function_name_hash("acos"):
    case promql_function_name_hash("acosh"):
    case promql_function_name_hash("asin"):
    case promql_function_name_hash("asinh"):
    case promql_function_name_hash("atan"):
    case promql_function_name_hash("atanh"):
    case promql_function_name_hash("avg_over_time"):
    case promql_function_name_hash("ceil"):
    case promql_function_name_hash("changes"):
    case promql_function_name_hash("clamp"):
    case promql_function_name_hash("clamp_max"):
    case promql_function_name_hash("clamp_min"):
    case promql_function_name_hash("cos"):
    case promql_function_name_hash("cosh"):
    case promql_function_name_hash("count_over_time"):
    case promql_function_name_hash("days_in_month"):
    case promql_function_name_hash("day_of_month"):
    case promql_function_name_hash("day_of_week"):
    case promql_function_name_hash("day_of_year"):
    case promql_function_name_hash("deg"):
    case promql_function_name_hash("deriv"):
    case promql_function_name_hash("exp"):
    case promql_function_name_hash("floor"):
    case promql_function_name_hash("histogram_avg"):
    case promql_function_name_hash("histogram_count"):
    case promql_function_name_hash("histogram_fraction"):
    case promql_function_name_hash("histogram_quantile"):
    case promql_function_name_hash("histogram_sum"):
    case promql_function_name_hash("histogram_stddev"):
    case promql_function_name_hash("histogram_stdvar"):
    case promql_function_name_hash("holt_winters"):
    case promql_function_name_hash("hour"):
    case promql_function_name_hash("info"):
    case promql_function_name_hash("label_replace"):
    case promql_function_name_hash("label_join"):
    case promql_function_name_hash("ln"):
    case promql_function_name_hash("log10"):
    case promql_function_name_hash("log2"):
    case promql_function_name_hash("mad_over_time"):
    case promql_function_name_hash("minute"):
    case promql_function_name_hash("month"):
    case promql_function_name_hash("pi"):
    case promql_function_name_hash("predict_linear"):
    case promql_function_name_hash("present_over_time"):
    case promql_function_name_hash("quantile_over_time"):
    case promql_function_name_hash("rad"):
    case promql_function_name_hash("round"):
    case promql_function_name_hash("scalar"):
    case promql_function_name_hash("sgn"):
    case promql_function_name_hash("sin"):
    case promql_function_name_hash("sinh"):
    case promql_function_name_hash("sort"):
    case promql_function_name_hash("sort_desc"):
    case promql_function_name_hash("sort_by_label"):
    case promql_function_name_hash("sort_by_label_desc"):
    case promql_function_name_hash("sqrt"):
    case promql_function_name_hash("stddev_over_time"):
    case promql_function_name_hash("stdvar_over_time"):
    case promql_function_name_hash("tan"):
    case promql_function_name_hash("tanh"):
    case promql_function_name_hash("time"):
    case promql_function_name_hash("timestamp"):
    case promql_function_name_hash("vector"):
    case promql_function_name_hash("year"):
    default:
      [[likely]] return DecodeIterator(std::in_place_type<DecodeIterator::UniversalDecodeIterator>);
  }
}

}  // namespace entrypoint::series_data