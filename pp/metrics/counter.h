#pragma once

#include <atomic>

#include "metric.h"

namespace metrics {

template <class Type = uint64_t>
class Counter final : public Metric {
 public:
  using Metric::Metric;

  Counter() = delete;
  explicit Counter(std::string_view name, std::integral auto value = 0) : Metric(name), value_(value) {}
  Counter(const Counter&) = delete;
  Counter(Counter&&) noexcept = delete;
  Counter& operator=(const Counter&) = delete;
  Counter& operator=(Counter&&) noexcept = delete;

  void serialize([[maybe_unused]] const LabelSet& labels, [[maybe_unused]] std::string& buffer) const override { ; }

  [[nodiscard]] size_t object_size() const noexcept override { return sizeof(*this); }

  PROMPP_ALWAYS_INLINE void Add(std::integral auto count) noexcept { value_ += count; }

 private:
  Type value_{};
};

template <class Type>
using AtomicCounter = Counter<std::atomic<Type>>;

// template <size_t BucketsCount>
// class Histogram final : public Metric {
// public:
//   using Metric::Metric;
//
//   void serialize([[maybe_unused]] const LabelSet& labels, [[maybe_unused]] std::string& buffer) const override { ; }
//
//   [[nodiscard]] size_t object_size() const noexcept override { return sizeof(*this); }
//
// private:
//   std::array<uint64_t, BucketsCount + 1> buckets_{};
// };

}  // namespace metrics