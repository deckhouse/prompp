#include <gtest/gtest.h>

#include "series_data/data_storage.h"
#include "series_data/decoder/decorator/window_function_iterator.h"
#include "series_data/encoder.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using PromPP::Primitives::TimeInterval;
using PromPP::Primitives::Timestamp;
using series_data::decoder::kInvalidTimestamp;
using series_data::decoder::decorator::StepRangeWindowCalculator;
using series_data::decoder::decorator::WindowBoundaryCalculatorInterface;
using series_data::decoder::decorator::WindowFunctionIteratorInterface;
using series_data::decoder::decorator::WindowFunctionParameters;
using series_data::encoder::Sample;

struct IntervalCalculatorCase {
  WindowFunctionParameters parameters;
  std::vector<TimeInterval> expected{};
};

template <WindowBoundaryCalculatorInterface Calculator>
class WindowBoundaryCalculatorFixture : public testing::TestWithParam<IntervalCalculatorCase> {
 protected:
  void test() {
    // Arrange
    std::vector<TimeInterval> actual;

    // Act
    actual.emplace_back(Calculator::initial_window(GetParam().parameters));
    std::generate_n(std::back_inserter(actual), GetParam().expected.size() - 1,
                    [&actual] { return Calculator::next_window(actual.back(), GetParam().parameters); });

    // Assert
    EXPECT_EQ(GetParam().expected, actual);
  }
};

using StepRangeWindowCalculatorFixture = WindowBoundaryCalculatorFixture<StepRangeWindowCalculator>;

TEST_P(StepRangeWindowCalculatorFixture, Test) {
  test();
}

INSTANTIATE_TEST_SUITE_P(NoInterval,
                         StepRangeWindowCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 0, .max = 0},
                                                                        .step = 0,
                                                                        .range = 100,
                                                                    },
                                                                .expected{TimeInterval{.min = 1, .max = 0}}}));

INSTANTIATE_TEST_SUITE_P(ZeroStep,
                         StepRangeWindowCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 0, .max = 1000},
                                                                        .step = 0,
                                                                        .range = 100,
                                                                    },
                                                                .expected{TimeInterval{.min = 1, .max = 1000}}}));

INSTANTIATE_TEST_SUITE_P(ZeroStepAndRange,
                         StepRangeWindowCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 0, .max = 1000},
                                                                        .step = 0,
                                                                        .range = 0,
                                                                    },
                                                                .expected{TimeInterval{.min = 1, .max = 1000}}}));

INSTANTIATE_TEST_SUITE_P(RangeEqualsStep,
                         StepRangeWindowCalculatorFixture,
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

INSTANTIATE_TEST_SUITE_P(RangeGreaterThanInterval,
                         StepRangeWindowCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 0, .max = 50},
                                                                        .step = 30,
                                                                        .range = 200,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 1, .max = 20},
                                                                    TimeInterval{.min = 21, .max = 30},
                                                                    TimeInterval{.min = 31, .max = 50},
                                                                }}));

INSTANTIATE_TEST_SUITE_P(StepGreaterThanInterval,
                         StepRangeWindowCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 0, .max = 50},
                                                                        .step = 60,
                                                                        .range = 0,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 1, .max = 50},
                                                                }}));

INSTANTIATE_TEST_SUITE_P(RangeAndStepGreaterThanInterval,
                         StepRangeWindowCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 0, .max = 50},
                                                                        .step = 60,
                                                                        .range = 70,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 1, .max = 10},
                                                                    TimeInterval{.min = 11, .max = 50},
                                                                }}));

INSTANTIATE_TEST_SUITE_P(RangeGreaterThanStepAndDivisibleIntervalCalculation,
                         StepRangeWindowCalculatorFixture,
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
                         StepRangeWindowCalculatorFixture,
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
                         StepRangeWindowCalculatorFixture,
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
                         StepRangeWindowCalculatorFixture,
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
                         StepRangeWindowCalculatorFixture,
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
                         StepRangeWindowCalculatorFixture,
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
                         StepRangeWindowCalculatorFixture,
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
                         StepRangeWindowCalculatorFixture,
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
                         StepRangeWindowCalculatorFixture,
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
                         StepRangeWindowCalculatorFixture,
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
                         StepRangeWindowCalculatorFixture,
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
                         StepRangeWindowCalculatorFixture,
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
                         StepRangeWindowCalculatorFixture,
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
                         StepRangeWindowCalculatorFixture,
                         testing::Values(IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 10, .max = 10},
                                                                        .step = 100,
                                                                        .range = 60,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 11, .max = 10},
                                                                }}));

INSTANTIATE_TEST_SUITE_P(RangeLargerThanStepNotDivisibleNoInvertedWindows,
                         StepRangeWindowCalculatorFixture,
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
                                                                }},
                                         IntervalCalculatorCase{.parameters =
                                                                    {
                                                                        .interval{.min = 0, .max = 32},
                                                                        .step = 10,
                                                                        .range = 12,
                                                                    },
                                                                .expected{
                                                                    TimeInterval{.min = 1, .max = 2},
                                                                    TimeInterval{.min = 3, .max = 10},
                                                                    TimeInterval{.min = 11, .max = 12},
                                                                    TimeInterval{.min = 13, .max = 20},
                                                                    TimeInterval{.min = 21, .max = 22},
                                                                    TimeInterval{.min = 23, .max = 30},
                                                                    TimeInterval{.min = 31, .max = 32},
                                                                }}));

}  // namespace