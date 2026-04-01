#include <gtest/gtest.h>

#include "series_data/data_storage.h"
#include "series_data/decoder/decorator/max_over_time.h"
#include "series_data/decoder/decorator/window_function_iterator.h"
#include "series_data/encoder.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using PromPP::Primitives::TimeInterval;
using PromPP::Primitives::Timestamp;
using series_data::DataStorage;
using series_data::Decoder;
using series_data::Encoder;
using series_data::chunk::DataChunk;
using series_data::decoder::DecodeIteratorSentinel;
using series_data::decoder::kInvalidTimestamp;
using series_data::decoder::SeekResult;
using series_data::decoder::UniversalDecodeIterator;
using series_data::decoder::decorator::MaxOverTimeIterator;
using series_data::decoder::decorator::WindowFunctionIterator;
using series_data::encoder::Sample;

struct WindowFunctionIteratorCase {
  std::vector<Sample> samples;
  TimeInterval interval;
  Timestamp step_ms{};
  Timestamp range_ms;
  std::vector<Sample> expected{};
};

template <class FunctionIterator>
class WindowFunctionIteratorFixture : public testing::TestWithParam<WindowFunctionIteratorCase> {
 protected:
  DataStorage storage_;

  void SetUp() override {
    Encoder encoder(storage_);
    for (const auto& sample : GetParam().samples) {
      encoder.encode(0, sample.timestamp, sample.value);
    }
  }

  void test() {
    // Arrange
    std::vector<Sample> actual_samples;

    // Act
    Decoder::create_decode_iterator<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0], [&actual_samples]<typename Iterator>(Iterator&& begin, auto&&) {
      std::ranges::copy(WindowFunctionIterator<FunctionIterator>(UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)},
                                                                 GetParam().interval, GetParam().step_ms, GetParam().range_ms),
                        DecodeIteratorSentinel{}, std::back_inserter(actual_samples));
    });

    // Assert
    EXPECT_EQ(GetParam().expected, actual_samples);
  }

  void test_reset() {
    // Arrange
    std::vector<Sample> actual_samples;

    // Act
    Decoder::create_decode_iterator<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0], [&actual_samples]<typename Iterator>(Iterator&& begin, auto&&) {
      WindowFunctionIterator<FunctionIterator> iterator(UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)},
                                                        GetParam().interval, GetParam().step_ms, GetParam().range_ms);
      std::advance(iterator, GetParam().samples.size());

      iterator = UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)};
      std::ranges::copy(iterator, DecodeIteratorSentinel{}, std::back_inserter(actual_samples));
    });

    // Assert
    EXPECT_EQ(GetParam().expected, actual_samples);
  }
};

using MaxOverTimeWindowFunctionIteratorFixture = WindowFunctionIteratorFixture<MaxOverTimeIterator>;

TEST_P(MaxOverTimeWindowFunctionIteratorFixture, Test) {
  test();
}

TEST_P(MaxOverTimeWindowFunctionIteratorFixture, TestReset) {
  test_reset();
}

INSTANTIATE_TEST_SUITE_P(NoStep,
                         MaxOverTimeWindowFunctionIteratorFixture,
                         testing::Values(WindowFunctionIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}},
                                                                    .interval{.min = 0, .max = 1000},
                                                                    .range_ms = 100,
                                                                    .expected{Sample{.timestamp = 100, .value = 1.0}}},
                                         WindowFunctionIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}, Sample{.timestamp = 1000, .value = 2.0}},
                                                                    .interval{.min = 0, .max = 1000},
                                                                    .range_ms = 100,
                                                                    .expected{Sample{.timestamp = 1000, .value = 2.0}}},
                                         WindowFunctionIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}, Sample{.timestamp = 500, .value = 2.0},
                                                                             Sample{.timestamp = 1000, .value = 1.0}},
                                                                    .interval{.min = 0, .max = 1000},
                                                                    .range_ms = 100,
                                                                    .expected{Sample{.timestamp = 500, .value = 2.0}}}));

INSTANTIATE_TEST_SUITE_P(StepGreaterThanRange,
                         MaxOverTimeWindowFunctionIteratorFixture,
                         testing::Values(WindowFunctionIteratorCase{
                             .samples{Sample{.timestamp = 130, .value = 1.0}, Sample{.timestamp = 160, .value = 2.0}, Sample{.timestamp = 190, .value = 3.0},
                                      Sample{.timestamp = 220, .value = 4.0}, Sample{.timestamp = 250, .value = 5.0}, Sample{.timestamp = 280, .value = 6.0},
                                      Sample{.timestamp = 310, .value = 7.0}},
                             .interval{.min = 0, .max = 1000},
                             .step_ms = 70,
                             .range_ms = 60,
                             .expected{Sample{.timestamp = 160, .value = 2.0}, Sample{.timestamp = 220, .value = 4.0}, Sample{.timestamp = 280, .value = 6.0},
                                       Sample{.timestamp = 310, .value = 7.0}}}));

INSTANTIATE_TEST_SUITE_P(StepLessThanRange,
                         MaxOverTimeWindowFunctionIteratorFixture,
                         testing::Values(WindowFunctionIteratorCase{
                             .samples{Sample{.timestamp = 130, .value = 1.0}, Sample{.timestamp = 160, .value = 2.0}, Sample{.timestamp = 190, .value = 3.0},
                                      Sample{.timestamp = 220, .value = 4.0}, Sample{.timestamp = 250, .value = 5.0}, Sample{.timestamp = 280, .value = 5.0},
                                      Sample{.timestamp = 310, .value = 6.0}},
                             .interval{.min = 80, .max = 310},
                             .step_ms = 60,
                             .range_ms = 70,
                             .expected{Sample{.timestamp = 130, .value = 1.0}, Sample{.timestamp = 190, .value = 3.0}, Sample{.timestamp = 250, .value = 5.0},
                                       Sample{.timestamp = 310, .value = 6.0}}}));

INSTANTIATE_TEST_SUITE_P(IntervalBoundaries,
                         MaxOverTimeWindowFunctionIteratorFixture,
                         testing::Values(WindowFunctionIteratorCase{.samples{Sample{.timestamp = 79, .value = 5.0}, Sample{.timestamp = 310, .value = 6.0}},
                                                                    .interval{.min = 80, .max = 309},
                                                                    .step_ms = 60,
                                                                    .range_ms = 70,
                                                                    .expected{}},
                                         WindowFunctionIteratorCase{.samples{Sample{.timestamp = 79, .value = 5.0}, Sample{.timestamp = 310, .value = 6.0}},
                                                                    .interval{.min = 0, .max = 79},
                                                                    .step_ms = 60,
                                                                    .range_ms = 70,
                                                                    .expected{Sample{.timestamp = 79, .value = 5.0}}},
                                         WindowFunctionIteratorCase{.samples{Sample{.timestamp = 79, .value = 5.0}, Sample{.timestamp = 310, .value = 6.0}},
                                                                    .interval{.min = 310, .max = 310},
                                                                    .step_ms = 60,
                                                                    .range_ms = 70,
                                                                    .expected{Sample{.timestamp = 310, .value = 6.0}}},
                                         WindowFunctionIteratorCase{.samples{Sample{.timestamp = 55, .value = 9.0}},
                                                                    .interval{.min = 0, .max = 50},
                                                                    .step_ms = 60,
                                                                    .range_ms = 70,
                                                                    .expected{}}));

INSTANTIATE_TEST_SUITE_P(StepEqualsRange,
                         MaxOverTimeWindowFunctionIteratorFixture,
                         testing::Values(WindowFunctionIteratorCase{
                             .samples{Sample{.timestamp = 50, .value = 1.0}, Sample{.timestamp = 150, .value = 4.0}, Sample{.timestamp = 250, .value = 2.0}},
                             .interval{.min = 0, .max = 300},
                             .step_ms = 100,
                             .range_ms = 100,
                             .expected{Sample{.timestamp = 50, .value = 1.0}, Sample{.timestamp = 150, .value = 4.0},
                                       Sample{.timestamp = 250, .value = 2.0}}}));

INSTANTIATE_TEST_SUITE_P(StaleNanSkipped,
                         MaxOverTimeWindowFunctionIteratorFixture,
                         testing::Values(WindowFunctionIteratorCase{.samples{Sample{.timestamp = 100, .value = STALE_NAN},
                                                                             Sample{.timestamp = 150, .value = 2.0}, Sample{.timestamp = 200, .value = 1.0}},
                                                                    .interval{.min = 0, .max = 400},
                                                                    .step_ms = 100,
                                                                    .range_ms = 100,
                                                                    .expected{Sample{.timestamp = 150, .value = 2.0}}}));

}  // namespace