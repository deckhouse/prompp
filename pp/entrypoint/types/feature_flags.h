#pragma once

#include <cstdint>

#include "feature_flags_constants.h"

namespace entrypoint::types {

enum class FeatureFlag : uint64_t {
  kScraperUtfPerToken = PROMPP_FEATURE_SCRAPER_UTF_PER_TOKEN,
};

class FeatureFlags {
 public:
  // Initialize before reading features; later calls are ignored.
  void initialize(uint64_t enabled_features) noexcept;
  [[nodiscard]] bool enabled(FeatureFlag feature) const noexcept;

 private:
  uint64_t enabled_features_{};
  bool initialized_{};
};

[[nodiscard]] FeatureFlags& feature_flags() noexcept;

}  // namespace entrypoint::types
