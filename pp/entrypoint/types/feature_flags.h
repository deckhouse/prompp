#pragma once

#include "feature_flags_config.h"

namespace entrypoint::types {

class FeatureFlags {
 public:
  // Initialize before reading features; later calls are ignored.
  void initialize(PromppFeatures features) noexcept;
  [[nodiscard]] const PromppFeatures& features() const noexcept { return features_; }

 private:
  PromppFeatures features_{};
  bool initialized_{};
};

[[nodiscard]] FeatureFlags& feature_flags() noexcept;

}  // namespace entrypoint::types
