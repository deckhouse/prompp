#include <gtest/gtest.h>

#include "series_data/decoder/decorator/stale_nan_deduplicate_iterator.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using series_data::decoder::decorator::StaleNanDeduplicateIterator;
using series_data::encoder::Sample;

struct StaleNanDeduplicateIteratorCase {
  std::vector<Sample> samples;
  std::vector<Sample> expected{};
};

class StaleNanDeduplicateIteratorFixture : public ::testing::TestWithParam<StaleNanDeduplicateIteratorCase> {};

TEST_P(StaleNanDeduplicateIteratorFixture, Test) {
  // Arrange
  std::vector<Sample> actual_samples;

  // Act
  std::ranges::copy(StaleNanDeduplicateIterator(GetParam().samples.begin(), GetParam().samples.end()), GetParam().samples.end(),
                    std::back_inserter(actual_samples));

  // Assert
  EXPECT_EQ(GetParam().expected, actual_samples);
}

INSTANTIATE_TEST_SUITE_P(Empty, StaleNanDeduplicateIteratorFixture, testing::Values(StaleNanDeduplicateIteratorCase{}));
INSTANTIATE_TEST_SUITE_P(NoStaleNans,
                         StaleNanDeduplicateIteratorFixture,
                         testing::Values(StaleNanDeduplicateIteratorCase{.samples =
                                                                             {
                                                                                 Sample{.timestamp = 1, .value = 1.0},
                                                                             },
                                                                         .expected =
                                                                             {
                                                                                 Sample{.timestamp = 1, .value = 1.0},
                                                                             }},
                                         StaleNanDeduplicateIteratorCase{.samples =
                                                                             {
                                                                                 Sample{.timestamp = 1, .value = 1.0},
                                                                                 Sample{.timestamp = 2, .value = 1.0},
                                                                                 Sample{.timestamp = 3, .value = 1.0},
                                                                             },
                                                                         .expected = {
                                                                             Sample{.timestamp = 1, .value = 1.0},
                                                                             Sample{.timestamp = 2, .value = 1.0},
                                                                             Sample{.timestamp = 3, .value = 1.0},
                                                                         }}));
INSTANTIATE_TEST_SUITE_P(StaleNans,
                         StaleNanDeduplicateIteratorFixture,
                         testing::Values(StaleNanDeduplicateIteratorCase{.samples =
                                                                             {
                                                                                 Sample{.timestamp = 1, .value = STALE_NAN},
                                                                             },
                                                                         .expected =
                                                                             {
                                                                                 Sample{.timestamp = 1, .value = STALE_NAN},
                                                                             }},
                                         StaleNanDeduplicateIteratorCase{.samples =
                                                                             {
                                                                                 Sample{.timestamp = 1, .value = STALE_NAN},
                                                                                 Sample{.timestamp = 2, .value = STALE_NAN},
                                                                                 Sample{.timestamp = 3, .value = STALE_NAN},
                                                                             },
                                                                         .expected =
                                                                             {
                                                                                 Sample{.timestamp = 1, .value = STALE_NAN},
                                                                             }},
                                         StaleNanDeduplicateIteratorCase{.samples =
                                                                             {
                                                                                 Sample{.timestamp = 1, .value = STALE_NAN},
                                                                                 Sample{.timestamp = 2, .value = STALE_NAN},
                                                                                 Sample{.timestamp = 3, .value = 1.0},
                                                                                 Sample{.timestamp = 4, .value = STALE_NAN},
                                                                                 Sample{.timestamp = 5, .value = STALE_NAN},
                                                                                 Sample{.timestamp = 6, .value = 2.0},
                                                                             },
                                                                         .expected = {
                                                                             Sample{.timestamp = 1, .value = STALE_NAN},
                                                                             Sample{.timestamp = 3, .value = 1.0},
                                                                             Sample{.timestamp = 4, .value = STALE_NAN},
                                                                             Sample{.timestamp = 6, .value = 2.0},
                                                                         }}));

}  // namespace