#pragma once

#include "bare_bones/xxhash.h"
#include "go_model.h"
#include "label_set.h"

namespace PromPP::Primitives::Go {

namespace dto {

struct MessageState {
  void* atomic_message_info{};
};

using UnknownFields = Slice<uint8_t>;
using CacheSize = int32_t;

struct LabelPair {
  PROMPP_ALWAYS_INLINE LabelPair(const String* n, const String* v) : name(n), value(v) {}

  MessageState state{};
  CacheSize size_cache{};
  UnknownFields unknown_fields{};

  const String* name;
  const String* value;
};

using LabelPairsList = std::vector<LabelPair>;

struct Gauge {
  explicit Gauge(double* counter) : value(counter) {}

  MessageState state{};
  CacheSize size_cache{};
  UnknownFields unknown_fields{};

  double* value{};
};

struct Counter {
  explicit Counter(double* counter) : value(counter) {}

  MessageState state{};
  CacheSize size_cache{};
  UnknownFields unknown_fields{};

  double* value{};
  void* exemplar{};
  void* created_timestamp{};
};

struct Metric {
  PROMPP_ALWAYS_INLINE Metric(const SliceView<const LabelPair*>& label_set, Gauge* gauge_ptr) : labels(label_set), gauge(gauge_ptr) {}
  PROMPP_ALWAYS_INLINE Metric(const SliceView<const LabelPair*>& label_set, Counter* counter_ptr) : labels(label_set), counter(counter_ptr) {}

  MessageState state{};
  CacheSize size_cache{};
  UnknownFields unknown_fields{};

  SliceView<const LabelPair*> labels{};
  Gauge* gauge{};
  Counter* counter{};
  void* summary{};
  void* untyped{};
  void* histogram{};
  void* timestamp{};
};

struct CompiledLabels {
  SliceView<String> names{};
  void* labelConstraints{};
};

struct MetricDescriptor {
  MetricDescriptor(String name, const LabelPairsList& labels, const CompiledLabels* variable_labels_ptr) : fq_name(name), variable_labels(variable_labels_ptr) {
    set_const_labels(labels);

    id = calculate_id();
    dim_hash = calculate_dim_hash();
  }

  String fq_name;
  String help{};
  Slice<const LabelPair*> const_label_pairs{};
  const CompiledLabels* variable_labels{};
  uint64_t id{};
  uint64_t dim_hash{};
  Error error{};

 private:
  static constexpr std::string_view kSeparator = "\xFF";

  void set_const_labels(const LabelPairsList& labels) {
    const_label_pairs.reserve(labels.size());
    for (const auto& label_pair : labels) {
      const_label_pairs.emplace_back(&label_pair);
    }
    std::ranges::sort(const_label_pairs, [](const LabelPair* a, const LabelPair* b) { return a->name < b->name; });
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t calculate_id() const noexcept {
    BareBones::XXHash hash;

    hash.extend(static_cast<std::string_view>(fq_name));
    hash.extend(kSeparator);

    for (const auto& label_pair : const_label_pairs) {
      hash.extend(static_cast<std::string_view>(*label_pair->value));
      hash.extend(kSeparator);
    }

    return hash.digest();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint64_t calculate_dim_hash() const noexcept {
    BareBones::XXHash hash;

    hash.extend(static_cast<std::string_view>(help));
    hash.extend(kSeparator);

    for (const auto& label_pair : const_label_pairs) {
      hash.extend(static_cast<std::string_view>(*label_pair->name));
      hash.extend(kSeparator);
    }

    return hash.digest();
  }
};

}  // namespace dto

struct Metric {
  dto::MetricDescriptor* descriptor{};
  dto::Metric* metric{};
};

}  // namespace PromPP::Primitives::Go