#include "feature_flags.h"

namespace entrypoint::types {

void FeatureFlags::initialize(const PromppFeatures features) noexcept {
  if (initialized_) {
    return;
  }

  features_ = features;
  initialized_ = true;
}

FeatureFlags& feature_flags() noexcept {
  static FeatureFlags flags;
  return flags;
}

}  // namespace entrypoint::types
