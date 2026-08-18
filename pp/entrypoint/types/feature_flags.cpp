#include "feature_flags.h"

namespace entrypoint::types {

void FeatureFlags::initialize(const uint64_t enabled_features) noexcept {
  if (initialized_) {
    return;
  }

  enabled_features_ = enabled_features;
  initialized_ = true;
}

bool FeatureFlags::enabled(const FeatureFlag feature) const noexcept {
  return (enabled_features_ & static_cast<uint64_t>(feature)) != 0;
}

FeatureFlags& feature_flags() noexcept {
  static FeatureFlags flags;
  return flags;
}

}  // namespace entrypoint::types
