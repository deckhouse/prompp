#pragma once

#include "bare_bones/concepts.h"
#include "primitives/go_metric.h"
#include "primitives/label_set.h"

namespace metrics {

class Metric {
 public:
  using Label = std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>;

  Metric() = delete;
  template <class LabelSet, class DtoMetricObject>
    requires std::same_as<DtoMetricObject, PromPP::Primitives::Go::dto::Gauge> || std::same_as<DtoMetricObject, PromPP::Primitives::Go::dto::Counter>
  Metric(const LabelSet& labels, std::string_view name, DtoMetricObject* object, size_t object_size)
      : labels_(labels, PromPP::Primitives::NoSortLabels{}),
        label_pairs_(create_label_pairs()),
        descriptor_(PromPP::Primitives::Go::String(name), label_pairs_, &variable_labels_),
        metric_(PromPP::Primitives::Go::SliceView(descriptor_.const_label_pairs), object),
        object_size_(object_size) {}
  Metric(const Metric&) = delete;
  Metric(Metric&&) noexcept = delete;
  Metric& operator=(const Metric&) = delete;
  Metric& operator=(Metric&&) noexcept = delete;

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t object_size() const noexcept { return object_size_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const PromPP::Primitives::Go::Metric* go_metric() const noexcept { return &go_metric_; }

 private:
  PromPP::Primitives::BasicLabelSet<Label> labels_;
  PromPP::Primitives::Go::dto::LabelPairsList label_pairs_;
  PromPP::Primitives::Go::dto::CompiledLabels variable_labels_;
  PromPP::Primitives::Go::dto::MetricDescriptor descriptor_;
  PromPP::Primitives::Go::dto::Metric metric_;
  PromPP::Primitives::Go::Metric go_metric_{.descriptor = &descriptor_, .metric = &metric_};
  const size_t object_size_{};

  PromPP::Primitives::Go::dto::LabelPairsList create_label_pairs() {
    PromPP::Primitives::Go::dto::LabelPairsList list;
    list.reserve(labels_.size());

    for (const auto& label : labels_) {
      list.emplace_back(&label.first, &label.second);
    }

    return list;
  }
};

template <class Type>
  requires std::same_as<Type, double> || std::same_as<Type, std::atomic<double>>
class GenericCounter final : public Metric {
 public:
  using Metric::Metric;

  template <class LabelSet, BareBones::concepts::arithmetic ValueType = double>
  explicit GenericCounter(LabelSet&& labels, std::string_view name, ValueType value = {})
      : Metric(std::forward<LabelSet>(labels), name, &counter_, sizeof(*this)), value_(value) {}

  PROMPP_ALWAYS_INLINE void inc(BareBones::concepts::arithmetic auto count) noexcept { value_ += count; }
  PROMPP_ALWAYS_INLINE void inc() noexcept { inc(1.0f); }

 protected:
  Type value_{};
  PromPP::Primitives::Go::dto::Counter counter_{reinterpret_cast<double*>(&value_)};
};

template <class Type>
  requires std::same_as<Type, double*> || std::same_as<Type, std::atomic_ref<double>>
class GenericCounterRef final : public Metric {
 public:
  using Metric::Metric;

  template <class LabelSet>
  explicit GenericCounterRef(LabelSet&& labels, std::string_view name, Type value)
      : Metric(std::forward<LabelSet>(labels), name, &counter_, sizeof(*this)), counter_(reinterpret_cast<double*>(value)) {}

 protected:
  PromPP::Primitives::Go::dto::Counter counter_;
};

using Counter = GenericCounter<double>;
using CounterRef = GenericCounterRef<double*>;
using AtomicCounter = GenericCounter<std::atomic<double>>;
using AtomicCounterRef = GenericCounterRef<std::atomic_ref<double>>;

class Gauge final : public Metric {
 public:
  using Metric::Metric;

  template <class LabelSet, BareBones::concepts::arithmetic ValueType = double>
  explicit Gauge(LabelSet&& labels, std::string_view name, ValueType value = {})
      : Metric(std::forward<LabelSet>(labels), name, &gauge_, sizeof(*this)), value_(value) {}

  PROMPP_ALWAYS_INLINE void inc(BareBones::concepts::arithmetic auto count) noexcept { value_ += count; }
  PROMPP_ALWAYS_INLINE void dec(BareBones::concepts::arithmetic auto count) noexcept { value_ -= count; }

 protected:
  double value_{};
  PromPP::Primitives::Go::dto::Gauge gauge_{&value_};
};

}  // namespace metrics