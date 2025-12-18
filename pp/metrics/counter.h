#pragma once

#include "bare_bones/concepts.h"
#include "metric.h"

namespace metrics {

class Counter final : public Metric {
 public:
  using Metric::Metric;

  template <class LabelSet, BareBones::concepts::arithmetic ValueType = double>
  explicit Counter(LabelSet&& labels, std::string_view name, ValueType value = {})
      : Metric(std::forward<LabelSet>(labels), name, &counter_, sizeof(*this)), value_(value) {}

  PROMPP_ALWAYS_INLINE void inc(BareBones::concepts::arithmetic auto count) noexcept { value_ += count; }
  PROMPP_ALWAYS_INLINE void dec(BareBones::concepts::arithmetic auto count) noexcept { value_ -= count; }

 private:
  double value_{};
  PromPP::Primitives::Go::dto::Counter counter_{&value_};
};

}  // namespace metrics