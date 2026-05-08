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
using series_data::decoder::decorator::WindowFunctionParameters;
using series_data::encoder::Sample;

struct WindowFunctionIteratorCase {
  std::vector<Sample> samples;
  WindowFunctionParameters parameters;
  std::vector<Sample> expected{};
};

template <class FunctionIterator>
class WindowFunctionIteratorFixture : public testing::TestWithParam<WindowFunctionIteratorCase> {
 protected:
  DataStorage storage_;
  Encoder<> encoder_{storage_};

  void encode_samples() {
    for (const auto& sample : GetParam().samples) {
      encoder_.encode(0, sample.timestamp, sample.value);
    }
  }

  void test() {
    // Arrange
    encode_samples();
    std::vector<Sample> actual_samples;

    // Act
    Decoder::create_decode_iterator<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0], [&actual_samples]<typename Iterator>(Iterator&& begin, auto&&) {
      std::ranges::copy(
          WindowFunctionIterator<FunctionIterator>(UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)}, GetParam().parameters),
          DecodeIteratorSentinel{}, std::back_inserter(actual_samples));
    });

    // Assert
    EXPECT_EQ(GetParam().expected, actual_samples);
  }

  void test_reset() {
    // Arrange
    encode_samples();
    std::vector<Sample> actual_samples;

    // Act
    Decoder::create_decode_iterator<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0], [&actual_samples]<typename Iterator>(Iterator&& begin, auto&&) {
      Iterator begin_at_start = begin;
      WindowFunctionIterator<FunctionIterator> iterator(UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)},
                                                        GetParam().parameters);
      std::advance(iterator, GetParam().samples.size());

      iterator = UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin_at_start)};
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

TEST_F(MaxOverTimeWindowFunctionIteratorFixture, TestContinueAfterReset) {
  // Arrange
  encoder_.encode(0, 130, 1.0);
  encoder_.encode(0, 160, 2.0);
  encoder_.encode(0, 190, 3.0);
  encoder_.encode(0, 220, 4.0);
  encoder_.encode(1, 250, 5.0);
  encoder_.encode(1, 280, 5.0);
  encoder_.encode(1, 310, 6.0);

  UniversalDecodeIterator it0;
  Decoder::create_decode_iterator<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[0], [&it0]<typename Iterator>(Iterator&& begin, auto&&) {
    it0 = UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)};
  });

  UniversalDecodeIterator it1;
  Decoder::create_decode_iterator<DataChunk::Type::kOpen>(storage_, storage_.open_chunks[1], [&it1]<typename Iterator>(Iterator&& begin, auto&&) {
    it1 = UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)};
  });

  constexpr WindowFunctionParameters parameters{
      .interval{.min = 80, .max = 310},
      .step = 60,
      .range = 70,
  };
  // NOLINTNEXTLINE(performance-move-const-arg)
  WindowFunctionIterator<MaxOverTimeIterator> iterator(std::move(it0), parameters);

  std::vector<Sample> actual_samples;

  // Act
  std::ranges::copy(iterator, DecodeIteratorSentinel{}, std::back_inserter(actual_samples));
  // NOLINTNEXTLINE(performance-move-const-arg)
  iterator = std::move(it1);
  std::ranges::copy(iterator, DecodeIteratorSentinel{}, std::back_inserter(actual_samples));

  // Assert
  EXPECT_EQ((std::vector{Sample{.timestamp = 130, .value = 1.0}, Sample{.timestamp = 190, .value = 3.0}, Sample{.timestamp = 220, .value = 4.0},
                         Sample{.timestamp = 250, .value = 5.0}, Sample{.timestamp = 310, .value = 6.0}}),
            actual_samples);
}

INSTANTIATE_TEST_SUITE_P(NoStep,
                         MaxOverTimeWindowFunctionIteratorFixture,
                         testing::Values(WindowFunctionIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}},
                                                                    .parameters =
                                                                        {
                                                                            .interval{.min = 0, .max = 1000},
                                                                            .step = 0,
                                                                            .range = 100,
                                                                        },
                                                                    .expected{Sample{.timestamp = 100, .value = 1.0}}},
                                         WindowFunctionIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}, Sample{.timestamp = 1000, .value = 2.0}},
                                                                    .parameters =
                                                                        {
                                                                            .interval{.min = 0, .max = 1000},
                                                                            .step = 0,
                                                                            .range = 100,
                                                                        },
                                                                    .expected{Sample{.timestamp = 1000, .value = 2.0}}},
                                         WindowFunctionIteratorCase{.samples{Sample{.timestamp = 100, .value = 1.0}, Sample{.timestamp = 500, .value = 2.0},
                                                                             Sample{.timestamp = 1000, .value = 1.0}},
                                                                    .parameters =
                                                                        {
                                                                            .interval{.min = 0, .max = 1000},
                                                                            .step = 0,
                                                                            .range = 100,
                                                                        },
                                                                    .expected{Sample{.timestamp = 500, .value = 2.0}}}));

INSTANTIATE_TEST_SUITE_P(NoRange,
                         MaxOverTimeWindowFunctionIteratorFixture,
                         testing::Values(WindowFunctionIteratorCase{
                             .samples{Sample{.timestamp = 100, .value = 1.0}, Sample{.timestamp = 1000, .value = 2.0}},
                             .parameters =
                                 {
                                     .interval{.min = 0, .max = 1000},
                                     .step = 100,
                                     .range = 0,
                                 },
                             .expected{Sample{.timestamp = 100, .value = 1.0}, Sample{.timestamp = 1000, .value = 2.0}}}));

INSTANTIATE_TEST_SUITE_P(StepGreaterThanRange,
                         MaxOverTimeWindowFunctionIteratorFixture,
                         testing::Values(WindowFunctionIteratorCase{
                             .samples{Sample{.timestamp = 130, .value = 1.0}, Sample{.timestamp = 160, .value = 2.0}, Sample{.timestamp = 190, .value = 3.0},
                                      Sample{.timestamp = 220, .value = 4.0}, Sample{.timestamp = 250, .value = 5.0}, Sample{.timestamp = 280, .value = 6.0},
                                      Sample{.timestamp = 310, .value = 7.0}},
                             .parameters =
                                 {
                                     .interval{.min = 0, .max = 1000},
                                     .step = 70,
                                     .range = 60,
                                 },
                             .expected{Sample{.timestamp = 160, .value = 2.0}, Sample{.timestamp = 220, .value = 4.0}, Sample{.timestamp = 280, .value = 6.0},
                                       Sample{.timestamp = 310, .value = 7.0}}}));

INSTANTIATE_TEST_SUITE_P(StepLessThanRange,
                         MaxOverTimeWindowFunctionIteratorFixture,
                         testing::Values(WindowFunctionIteratorCase{
                             .samples{Sample{.timestamp = 130, .value = 1.0}, Sample{.timestamp = 160, .value = 2.0}, Sample{.timestamp = 190, .value = 3.0},
                                      Sample{.timestamp = 220, .value = 4.0}, Sample{.timestamp = 250, .value = 5.0}, Sample{.timestamp = 280, .value = 5.0},
                                      Sample{.timestamp = 310, .value = 6.0}},
                             .parameters =
                                 {
                                     .interval{.min = 80, .max = 310},
                                     .step = 60,
                                     .range = 70,
                                 },
                             .expected{Sample{.timestamp = 130, .value = 1.0}, Sample{.timestamp = 190, .value = 3.0}, Sample{.timestamp = 250, .value = 5.0},
                                       Sample{.timestamp = 310, .value = 6.0}}}));

INSTANTIATE_TEST_SUITE_P(IntervalBoundaries,
                         MaxOverTimeWindowFunctionIteratorFixture,
                         testing::Values(WindowFunctionIteratorCase{.samples{Sample{.timestamp = 79, .value = 5.0}, Sample{.timestamp = 310, .value = 6.0}},
                                                                    .parameters =
                                                                        {
                                                                            .interval{.min = 80, .max = 309},
                                                                            .step = 60,
                                                                            .range = 70,
                                                                        },
                                                                    .expected{}},
                                         WindowFunctionIteratorCase{.samples{Sample{.timestamp = 79, .value = 5.0}, Sample{.timestamp = 310, .value = 6.0}},
                                                                    .parameters =
                                                                        {
                                                                            .interval{.min = 0, .max = 79},
                                                                            .step = 60,
                                                                            .range = 70,
                                                                        },
                                                                    .expected{Sample{.timestamp = 79, .value = 5.0}}},
                                         WindowFunctionIteratorCase{.samples{Sample{.timestamp = 79, .value = 5.0}, Sample{.timestamp = 310, .value = 6.0}},
                                                                    .parameters =
                                                                        {
                                                                            .interval{.min = 310, .max = 310},
                                                                            .step = 60,
                                                                            .range = 70,
                                                                        },
                                                                    .expected{Sample{.timestamp = 310, .value = 6.0}}},
                                         WindowFunctionIteratorCase{.samples{Sample{.timestamp = 55, .value = 9.0}},
                                                                    .parameters =
                                                                        {
                                                                            .interval{.min = 0, .max = 50},
                                                                            .step = 60,
                                                                            .range = 70,
                                                                        },
                                                                    .expected{}}));

INSTANTIATE_TEST_SUITE_P(StepEqualsRange,
                         MaxOverTimeWindowFunctionIteratorFixture,
                         testing::Values(WindowFunctionIteratorCase{
                             .samples{Sample{.timestamp = 50, .value = 1.0}, Sample{.timestamp = 150, .value = 4.0}, Sample{.timestamp = 250, .value = 2.0}},
                             .parameters =
                                 {
                                     .interval{.min = 0, .max = 300},
                                     .step = 100,
                                     .range = 100,
                                 },
                             .expected{Sample{.timestamp = 50, .value = 1.0}, Sample{.timestamp = 150, .value = 4.0},
                                       Sample{.timestamp = 250, .value = 2.0}}}));

INSTANTIATE_TEST_SUITE_P(StaleNanSkipped,
                         MaxOverTimeWindowFunctionIteratorFixture,
                         testing::Values(WindowFunctionIteratorCase{.samples{Sample{.timestamp = 100, .value = STALE_NAN},
                                                                             Sample{.timestamp = 150, .value = 2.0}, Sample{.timestamp = 200, .value = 1.0}},
                                                                    .parameters =
                                                                        {
                                                                            .interval{.min = 0, .max = 400},
                                                                            .step = 100,
                                                                            .range = 100,
                                                                        },
                                                                    .expected{Sample{.timestamp = 150, .value = 2.0}}}));

}  // namespace