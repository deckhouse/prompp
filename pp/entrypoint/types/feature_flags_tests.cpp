#include <gtest/gtest.h>

#include "feature_flags.h"

namespace {

using entrypoint::types::FeatureFlags;
TEST(FeatureFlags, FeaturesAreDisabledBeforeInitialization) {
  // Arrange
  const FeatureFlags flags;

  // Act
  const auto enabled = flags.features().scraper_validate_utf_per_token;

  // Assert
  EXPECT_FALSE(enabled);
}

TEST(FeatureFlags, InitializesEnabledFeatures) {
  // Arrange
  FeatureFlags flags;

  // Act
  flags.initialize(PromppFeatures{.scraper_validate_utf_per_token = true});

  // Assert
  EXPECT_TRUE(flags.features().scraper_validate_utf_per_token);
}

TEST(FeatureFlags, EmptyInitializationKeepsFeaturesDisabled) {
  // Arrange
  FeatureFlags flags;

  // Act
  flags.initialize({});

  // Assert
  EXPECT_FALSE(flags.features().scraper_validate_utf_per_token);
}

TEST(FeatureFlags, RepeatedInitializationIsIgnored) {
  // Arrange
  FeatureFlags flags;
  flags.initialize(PromppFeatures{.scraper_validate_utf_per_token = true});

  // Act
  flags.initialize({});

  // Assert
  EXPECT_TRUE(flags.features().scraper_validate_utf_per_token);
}

}  // namespace
