#pragma once

#include <atomic>

#include "metric.h"
#include "serializer.h"

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

  void serialize(protozero::pbf_writer& writer) const override {
    enum Tag : int {
      kValue = 1,
    };

    protozero::pbf_writer counter_writer(writer, Serializer::Tag::kCounter);
    counter_writer.add_double(Tag::kValue, static_cast<double>(value_));
  }

  [[nodiscard]] size_t object_size() const noexcept override { return sizeof(*this); }

  PROMPP_ALWAYS_INLINE void add(std::integral auto count) noexcept { value_ += count; }

 private:
  Type value_{};
};

template <class Type>
using AtomicCounter = Counter<std::atomic<Type>>;

}  // namespace metrics