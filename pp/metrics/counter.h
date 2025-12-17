#pragma once

#include "bare_bones/concepts.h"
#include "metric.h"

namespace metrics {

class Counter final : public Metric {
 public:
  using Metric::Metric;

  Counter() = delete;
  template <class LabelSet, BareBones::concepts::arithmetic ValueType = double>
  explicit Counter(LabelSet&& labels, std::string_view name, ValueType value = {}) : Metric(std::forward<LabelSet>(labels), name, &counter_), value_(value) {}
  Counter(const Counter&) = delete;
  Counter(Counter&&) noexcept = delete;
  Counter& operator=(const Counter&) = delete;
  Counter& operator=(Counter&&) noexcept = delete;

  [[nodiscard]] size_t object_size() const noexcept override { return sizeof(*this); }

  PROMPP_ALWAYS_INLINE void inc(BareBones::concepts::arithmetic auto count) noexcept { value_ += count; }
  PROMPP_ALWAYS_INLINE void dec(BareBones::concepts::arithmetic auto count) noexcept { value_ -= count; }

 private:
  double value_{};
  PromPP::Primitives::Go::dto::Counter counter_{&value_};
};

}  // namespace metrics