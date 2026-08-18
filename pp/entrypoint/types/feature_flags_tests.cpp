#include <gtest/gtest.h>

#include "feature_flags.h"

namespace {

using entrypoint::types::FeatureFlag;
using entrypoint::types::FeatureFlags;
TEST(FeatureFlags, FeaturesAreDisabledBeforeInitialization) {
  // Arrange
  const FeatureFlags flags;

  // Act
  const auto enabled = flags.enabled(FeatureFlag::kScraperUtfPerToken);

  // Assert
  EXPECT_FALSE(enabled);
}

TEST(FeatureFlags, InitializesEnabledFeatures) {
  // Arrange
  FeatureFlags flags;

  // Act
  flags.initialize(PROMPP_FEATURE_SCRAPER_UTF_PER_TOKEN);

  // Assert
  EXPECT_TRUE(flags.enabled(FeatureFlag::kScraperUtfPerToken));
}

TEST(FeatureFlags, EmptyInitializationKeepsFeaturesDisabled) {
  // Arrange
  FeatureFlags flags;

  // Act
  flags.initialize(0);

  // Assert
  EXPECT_FALSE(flags.enabled(FeatureFlag::kScraperUtfPerToken));
}

TEST(FeatureFlags, RepeatedInitializationIsIgnored) {
  // Arrange
  FeatureFlags flags;
  flags.initialize(PROMPP_FEATURE_SCRAPER_UTF_PER_TOKEN);

  // Act
  flags.initialize(0);

  // Assert
  EXPECT_TRUE(flags.enabled(FeatureFlag::kScraperUtfPerToken));
}

}  // namespace
