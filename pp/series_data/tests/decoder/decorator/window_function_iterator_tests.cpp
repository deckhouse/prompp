#include <gtest/gtest.h>

#include "series_data/data_storage.h"
#include "series_data/decoder/decorator/max_over_time.h"
#include "series_data/decoder/decorator/sum_over_time.h"
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
using series_data::decoder::decorator::SumOverTimeIterator;
using series_data::decoder::decorator::WindowBoundaryCalculator;
using series_data::decoder::decorator::WindowFunctionIterator;
using series_data::decoder::decorator::WindowFunctionIteratorInterface;
using series_data::decoder::decorator::WindowFunctionParameters;
using series_data::encoder::Sample;

struct IntervalCalculatorCase {
  WindowFunctionParameters parameters;
  std::vector<TimeInterval> expected{};
};

class WindowBoundaryCalculatorFixture : public testing::TestWithParam<IntervalCalculatorCase> {};

TEST_P(WindowBoundaryCalculatorFixture, Test) {
  // Arrange
  std::vector<TimeInterval> actual;

  // Act
  actual.emplace_back(WindowBoundaryCalculator::initial_window(GetParam().parameters));
  std::generate_n(std::back_inserter(actual), GetParam().expected.size() - 1,
                  [&actual] { return WindowBoundaryCalculator::next_window(actual.back(), GetParam().parameters); });

  // Assert
  EXPECT_EQ(GetParam().expected, actual);
}

INSTANTIATE_TEST_SUITE_P(NoInterval,
                         WindowBoundaryCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 0, .max = 0},
                                                                        .step = 0,
                                                                        .range = 100,
                                                                    },
                                                                .expected{TimeInterval{.min = 1, .max = 0}}}));

INSTANTIATE_TEST_SUITE_P(RangeEqualsStep,
                         WindowBoundaryCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 0, .max = 300},
                                                                        .step = 100,
                                                                        .range = 100,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 1, .max = 100},
                                                                    TimeInterval{.min = 101, .max = 200},
                                                                    TimeInterval{.min = 201, .max = 300},
                                                                }}));

INSTANTIATE_TEST_SUITE_P(RangeGreaterThanStepAndDivisibleIntervalCalculation,
                         WindowBoundaryCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 0, .max = 300},
                                                                        .step = 60,
                                                                        .range = 120,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 1, .max = 60},
                                                                    TimeInterval{.min = 61, .max = 120},
                                                                    TimeInterval{.min = 121, .max = 180},
                                                                    TimeInterval{.min = 181, .max = 240},
                                                                    TimeInterval{.min = 241, .max = 300},
                                                                }}));

INSTANTIATE_TEST_SUITE_P(RangeGreaterThanStepAndNotDivisible,
                         WindowBoundaryCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 80, .max = 310},
                                                                        .step = 60,
                                                                        .range = 70,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 81, .max = 90},
                                                                    TimeInterval{.min = 91, .max = 140},
                                                                    TimeInterval{.min = 141, .max = 150},
                                                                    TimeInterval{.min = 151, .max = 200},
                                                                    TimeInterval{.min = 201, .max = 210},
                                                                    TimeInterval{.min = 211, .max = 260},
                                                                    TimeInterval{.min = 261, .max = 270},
                                                                    TimeInterval{.min = 271, .max = 310},
                                                                }}));

INSTANTIATE_TEST_SUITE_P(RangeLessThanStepIntervalCalculation,
                         WindowBoundaryCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 0, .max = 300},
                                                                        .step = 100,
                                                                        .range = 60,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 1, .max = 60},
                                                                    TimeInterval{.min = 101, .max = 160},
                                                                    TimeInterval{.min = 201, .max = 260},
                                                                }}));

INSTANTIATE_TEST_SUITE_P(RangeEqualsStepTruncatedLastChunk,
                         WindowBoundaryCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 0, .max = 120},
                                                                        .step = 50,
                                                                        .range = 50,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 1, .max = 50},
                                                                    TimeInterval{.min = 51, .max = 100},
                                                                    TimeInterval{.min = 101, .max = 120},
                                                                }}));

INSTANTIATE_TEST_SUITE_P(RangeEqualsStepSingleChunk,
                         WindowBoundaryCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 0, .max = 50},
                                                                        .step = 50,
                                                                        .range = 50,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 1, .max = 50},
                                                                }}));

INSTANTIATE_TEST_SUITE_P(RangeZeroStepAlignedChunks,
                         WindowBoundaryCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 0, .max = 1000},
                                                                        .step = 100,
                                                                        .range = 0,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 1, .max = 100},
                                                                    TimeInterval{.min = 101, .max = 200},
                                                                    TimeInterval{.min = 201, .max = 300},
                                                                    TimeInterval{.min = 301, .max = 400},
                                                                    TimeInterval{.min = 401, .max = 500},
                                                                    TimeInterval{.min = 501, .max = 600},
                                                                    TimeInterval{.min = 601, .max = 700},
                                                                    TimeInterval{.min = 701, .max = 800},
                                                                    TimeInterval{.min = 801, .max = 900},
                                                                    TimeInterval{.min = 901, .max = 1000},
                                                                }}));

INSTANTIATE_TEST_SUITE_P(RangeGreaterThanStepDivisibleWithNonZeroCHints,
                         WindowBoundaryCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 10, .max = 130},
                                                                        .step = 40,
                                                                        .range = 120,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 11, .max = 50},
                                                                    TimeInterval{.min = 51, .max = 90},
                                                                    TimeInterval{.min = 91, .max = 130},
                                                                }}));

INSTANTIATE_TEST_SUITE_P(RangeGreaterThanStepDivisibleTruncatedByEnd,
                         WindowBoundaryCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 0, .max = 250},
                                                                        .step = 40,
                                                                        .range = 120,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 1, .max = 40},
                                                                    TimeInterval{.min = 41, .max = 80},
                                                                    TimeInterval{.min = 81, .max = 120},
                                                                    TimeInterval{.min = 121, .max = 160},
                                                                    TimeInterval{.min = 161, .max = 200},
                                                                    TimeInterval{.min = 201, .max = 240},
                                                                    TimeInterval{.min = 241, .max = 250},
                                                                }}));

INSTANTIATE_TEST_SUITE_P(RangeGreaterThanStepNotDivisibleFromZero,
                         WindowBoundaryCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 0, .max = 50},
                                                                        .step = 10,
                                                                        .range = 15,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 1, .max = 5},
                                                                    TimeInterval{.min = 6, .max = 10},
                                                                    TimeInterval{.min = 11, .max = 15},
                                                                    TimeInterval{.min = 16, .max = 20},
                                                                    TimeInterval{.min = 21, .max = 25},
                                                                    TimeInterval{.min = 26, .max = 30},
                                                                    TimeInterval{.min = 31, .max = 35},
                                                                    TimeInterval{.min = 36, .max = 40},
                                                                    TimeInterval{.min = 41, .max = 45},
                                                                    TimeInterval{.min = 46, .max = 50},
                                                                }}));

INSTANTIATE_TEST_SUITE_P(RangeGreaterThanStepNotDivisibleWithNonZeroCHints,
                         WindowBoundaryCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 100, .max = 200},
                                                                        .step = 30,
                                                                        .range = 50,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 101, .max = 120},
                                                                    TimeInterval{.min = 121, .max = 130},
                                                                    TimeInterval{.min = 131, .max = 150},
                                                                    TimeInterval{.min = 151, .max = 160},
                                                                    TimeInterval{.min = 161, .max = 180},
                                                                    TimeInterval{.min = 181, .max = 190},
                                                                    TimeInterval{.min = 191, .max = 200},
                                                                }}));

INSTANTIATE_TEST_SUITE_P(RangeLessThanStepWithNonZeroCHints,
                         WindowBoundaryCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 50, .max = 350},
                                                                        .step = 100,
                                                                        .range = 60,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 51, .max = 110},
                                                                    TimeInterval{.min = 151, .max = 210},
                                                                    TimeInterval{.min = 251, .max = 310},
                                                                }}));

INSTANTIATE_TEST_SUITE_P(RangeLessThanStepTruncatedLastWindow,
                         WindowBoundaryCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 0, .max = 250},
                                                                        .step = 100,
                                                                        .range = 60,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 1, .max = 60},
                                                                    TimeInterval{.min = 101, .max = 160},
                                                                    TimeInterval{.min = 201, .max = 250},
                                                                }}));

INSTANTIATE_TEST_SUITE_P(RangeLessThanStepSinglePartialWindow,
                         WindowBoundaryCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 0, .max = 40},
                                                                        .step = 100,
                                                                        .range = 60,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 1, .max = 40},
                                                                }}));

INSTANTIATE_TEST_SUITE_P(PointQueryIntervalWithWinLessThanStep,
                         WindowBoundaryCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 10, .max = 10},
                                                                        .step = 100,
                                                                        .range = 60,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 11, .max = 10},
                                                                }}));

INSTANTIATE_TEST_SUITE_P(RangeMuchLargerThanStepNotDivisibleNoInvertedWindows,
                         WindowBoundaryCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 0, .max = 20},
                                                                        .step = 4,
                                                                        .range = 9,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 1, .max = 1},
                                                                    TimeInterval{.min = 2, .max = 4},
                                                                    TimeInterval{.min = 5, .max = 5},
                                                                    TimeInterval{.min = 6, .max = 8},
                                                                    TimeInterval{.min = 9, .max = 9},
                                                                    TimeInterval{.min = 10, .max = 12},
                                                                    TimeInterval{.min = 13, .max = 13},
                                                                    TimeInterval{.min = 14, .max = 16},
                                                                    TimeInterval{.min = 17, .max = 17},
                                                                    TimeInterval{.min = 18, .max = 20},
                                                                }}));

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
                             .samples{Sample{.timestamp = 100, .value = 3.0}, Sample{.timestamp = 1000, .value = 2.0}},
                             .parameters =
                                 {
                                     .interval{.min = 0, .max = 1000},
                                     .step = 100,
                                     .range = 0,
                                 },
                             .expected{Sample{.timestamp = 100, .value = 3.0}, Sample{.timestamp = 1000, .value = 2.0}}}));

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
                             .expected{Sample{.timestamp = 130, .value = 1.0}, Sample{.timestamp = 190, .value = 3.0}, Sample{.timestamp = 250, .value = 5.0},
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
                                                                    .expected{}},
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

using SumOverTimeWindowFunctionIteratorFixture = WindowFunctionIteratorFixture<SumOverTimeIterator>;

TEST_P(SumOverTimeWindowFunctionIteratorFixture, Test) {
  test();
}

TEST_P(SumOverTimeWindowFunctionIteratorFixture, TestReset) {
  test_reset();
}

INSTANTIATE_TEST_SUITE_P(DoesNotDoubleCountBoundarySample,
                         SumOverTimeWindowFunctionIteratorFixture,
                         testing::Values(WindowFunctionIteratorCase{
                             .samples{Sample{.timestamp = 100, .value = 10.0}, Sample{.timestamp = 150, .value = 1.0}, Sample{.timestamp = 200, .value = 2.0}},
                             .parameters =
                                 {
                                     .interval{.min = 0, .max = 200},
                                     .step = 100,
                                     .range = 1000,
                                 },
                             .expected{Sample{.timestamp = 100, .value = 10.0}, Sample{.timestamp = 200, .value = 3.0}}}));

}  // namespace