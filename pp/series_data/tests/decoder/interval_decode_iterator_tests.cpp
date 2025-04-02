#include <gtest/gtest.h>

#include "series_data/decoder/interval_decode_iterator.h"

namespace {

using series_data::decoder::IntervalDecodeIterator;
using series_data::encoder::Sample;

struct IntervalDecodeIteratorCase {
  std::vector<Sample> samples;
  PromPP::Primitives::Timestamp interval;
  PromPP::Primitives::Timestamp lookback_delta{1000};
  std::vector<Sample> expected{};
};

class IntervalDecodeIteratorFixture : public ::testing::TestWithParam<IntervalDecodeIteratorCase> {};

TEST_P(IntervalDecodeIteratorFixture, Test) {
  // Arrange
  std::vector<Sample> actual_samples;

  // Act
  std::ranges::copy(IntervalDecodeIterator(GetParam().samples.begin(), GetParam().samples.end(), GetParam().interval, GetParam().lookback_delta),
                    GetParam().samples.end(), std::back_inserter(actual_samples));

  // Assert
  EXPECT_EQ(GetParam().expected, actual_samples);
}

INSTANTIATE_TEST_SUITE_P(Empty, IntervalDecodeIteratorFixture, testing::Values(IntervalDecodeIteratorCase{}));
INSTANTIATE_TEST_SUITE_P(
    OneSample,
    IntervalDecodeIteratorFixture,
    testing::Values(
        IntervalDecodeIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}}, .interval = 100, .expected{Sample{.timestamp = 100, .value = 1.0}}},
        IntervalDecodeIteratorCase{.samples{Sample{.timestamp = 300, .value = 1.0}}, .interval = 400, .expected{Sample{.timestamp = 300, .value = 1.0}}}));
INSTANTIATE_TEST_SUITE_P(ManySamples,
                         IntervalDecodeIteratorFixture,
                         testing::Values(
                             IntervalDecodeIteratorCase{
                                 .samples{
                                     Sample{.timestamp = 100, .value = 1.0},
                                     Sample{.timestamp = 200, .value = 1.0},
                                     Sample{.timestamp = 300, .value = 1.0},
                                 },
                                 .interval = 100,
                                 .expected{
                                     Sample{.timestamp = 100, .value = 1.0},
                                     Sample{.timestamp = 200, .value = 1.0},
                                     Sample{.timestamp = 300, .value = 1.0},
                                 },
                             },
                             IntervalDecodeIteratorCase{
                                 .samples{
                                     Sample{.timestamp = 100, .value = 1.0},
                                     Sample{.timestamp = 150, .value = 1.0},
                                     Sample{.timestamp = 200, .value = 1.0},
                                 },
                                 .interval = 100,
                                 .expected{
                                     Sample{.timestamp = 100, .value = 1.0},
                                     Sample{.timestamp = 200, .value = 1.0},
                                 },
                             },
                             IntervalDecodeIteratorCase{
                                 .samples{
                                     Sample{.timestamp = 123, .value = 1.0},
                                     Sample{.timestamp = 152, .value = 1.0},
                                     Sample{.timestamp = 180, .value = 1.0},
                                     Sample{.timestamp = 215, .value = 1.0},
                                     Sample{.timestamp = 242, .value = 1.0},
                                     Sample{.timestamp = 275, .value = 1.0},
                                     Sample{.timestamp = 303, .value = 1.0},
                                 },
                                 .interval = 100,
                                 .expected{
                                     Sample{.timestamp = 180, .value = 1.0},
                                     Sample{.timestamp = 275, .value = 1.0},
                                     Sample{.timestamp = 303, .value = 1.0},
                                 },
                             },
                             IntervalDecodeIteratorCase{
                                 .samples{
                                     Sample{.timestamp = 180, .value = 1.0},
                                     Sample{.timestamp = 275, .value = 1.0},
                                     Sample{.timestamp = 503, .value = 1.0},
                                     Sample{.timestamp = 603, .value = 1.0},
                                     Sample{.timestamp = 604, .value = 1.0},
                                 },
                                 .interval = 100,
                                 .expected{
                                     Sample{.timestamp = 180, .value = 1.0},
                                     Sample{.timestamp = 275, .value = 1.0},
                                     Sample{.timestamp = 275, .value = 1.0},
                                     Sample{.timestamp = 275, .value = 1.0},
                                     Sample{.timestamp = 503, .value = 1.0},
                                     Sample{.timestamp = 604, .value = 1.0},
                                 },
                             }));
INSTANTIATE_TEST_SUITE_P(UseMinInterval,
                         IntervalDecodeIteratorFixture,
                         testing::Values(IntervalDecodeIteratorCase{.samples{
                                                                        Sample{.timestamp = 0, .value = 1.0},
                                                                        Sample{.timestamp = 1, .value = 1.0},
                                                                        Sample{.timestamp = 2, .value = 1.0},
                                                                    },
                                                                    .interval = 0,
                                                                    .expected{
                                                                        Sample{.timestamp = 0, .value = 1.0},
                                                                        Sample{.timestamp = 1, .value = 1.0},
                                                                        Sample{.timestamp = 2, .value = 1.0},
                                                                    }}));
INSTANTIATE_TEST_SUITE_P(LookbackDelta,
                         IntervalDecodeIteratorFixture,
                         testing::Values(
                             IntervalDecodeIteratorCase{
                                 .samples{
                                     Sample{.timestamp = 180, .value = 1.0},
                                     Sample{.timestamp = 275, .value = 1.0},
                                     Sample{.timestamp = 503, .value = 1.0},
                                     Sample{.timestamp = 603, .value = 1.0},
                                 },
                                 .interval = 100,
                                 .lookback_delta = 125,
                                 .expected{
                                     Sample{.timestamp = 180, .value = 1.0},
                                     Sample{.timestamp = 275, .value = 1.0},
                                     Sample{.timestamp = 275, .value = 1.0},
                                     Sample{.timestamp = 503, .value = 1.0},
                                     Sample{.timestamp = 603, .value = 1.0},
                                 },
                             },
                             IntervalDecodeIteratorCase{
                                 .samples{
                                     Sample{.timestamp = 180, .value = 1.0},
                                     Sample{.timestamp = 275, .value = 1.0},
                                     Sample{.timestamp = 503, .value = 1.0},
                                     Sample{.timestamp = 603, .value = 1.0},
                                 },
                                 .interval = 100,
                                 .lookback_delta = 124,
                                 .expected{
                                     Sample{.timestamp = 180, .value = 1.0},
                                     Sample{.timestamp = 275, .value = 1.0},
                                     Sample{.timestamp = 503, .value = 1.0},
                                     Sample{.timestamp = 603, .value = 1.0},
                                 },
                             },
                             IntervalDecodeIteratorCase{
                                 .samples{
                                     Sample{.timestamp = 1, .value = 1.0},
                                 },
                                 .interval = 101,
                                 .lookback_delta = 100,
                                 .expected{
                                     Sample{.timestamp = 1, .value = 1.0},
                                 },
                             }));
INSTANTIATE_TEST_SUITE_P(NoSamples,
                         IntervalDecodeIteratorFixture,
                         testing::Values(IntervalDecodeIteratorCase{.samples{Sample{.timestamp = 1, .value = 1.0}}, .interval = 100, .lookback_delta = 98},
                                         IntervalDecodeIteratorCase{.samples{
                                                                        Sample{.timestamp = 1, .value = 1.0},
                                                                        Sample{.timestamp = 2, .value = 1.0},
                                                                        Sample{.timestamp = 3, .value = 1.0},
                                                                    },
                                                                    .interval = 100,
                                                                    .lookback_delta = 96}));

}  // namespace