#pragma once

#include "primitives/label_set.h"
#include "prometheus/value.h"

namespace metrics {

class LabelSet {
 public:
  template <class AnyLabelSet>
    requires(!std::is_same_v<AnyLabelSet, LabelSet>)
  explicit LabelSet(AnyLabelSet&& label_set) : labels_(std::forward<AnyLabelSet>(label_set)) {
    add_metric_name_label();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const PromPP::Primitives::LabelSet& labels() const noexcept { return labels_; }
  PROMPP_ALWAYS_INLINE void set_name(const std::string_view& name) const { *name_ = name; }

 private:
  static constexpr PromPP::Primitives::Label::second_type kEmptyName = "_";

  PromPP::Primitives::LabelSet labels_;
  PromPP::Primitives::Label::second_type* name_{};

  void add_metric_name_label() { name_ = &labels_.add(PromPP::Primitives::Label{PromPP::Prometheus::kMetricLabelName, kEmptyName})->second; }
};

}  // namespace metrics