#include <gtest/gtest.h>

#include <algorithm>
#include <cctype>

#include "primitives/histogram.h"
#include "primitives/label_set.h"

namespace {

using PromPP::Primitives::BasicHistogram;
using PromPP::Primitives::ClassicHistogramBucket;
using PromPP::Primitives::HistogramBucketValue;
using PromPP::Primitives::HistogramSpan;
using PromPP::Primitives::HistogramType;
using PromPP::Primitives::HistogramValue;
using PromPP::Primitives::HistogramValueType;
using PromPP::Primitives::kCustomBucketsSchema;

using Histogram = BasicHistogram<BareBones::Vector, BareBones::Vector, BareBones::Vector>;

constexpr double kInf = std::numeric_limits<double>::infinity();

struct ConvertCase {
  HistogramType input_type{};
  HistogramValue input_count{};
  double sum{0.0};
  std::vector<ClassicHistogramBucket> classic_buckets;
  Histogram expected;
};

[[nodiscard]] ClassicHistogramBucket uint_bucket(double upper, uint64_t cumulative) {
  return {.cumulative_count = {.value = cumulative}, .upper_bound = upper, .type = HistogramValueType::kUint};
}

[[nodiscard]] ClassicHistogramBucket float_bucket(double upper, double cumulative) {
  return {.cumulative_count = {.float_value = cumulative}, .upper_bound = upper, .type = HistogramValueType::kFloat};
}

class ConvertClassicToNativeHistogramFixture : public ::testing::TestWithParam<ConvertCase> {};

TEST_P(ConvertClassicToNativeHistogramFixture, Test) {
  // Arrange
  const auto& test_case = GetParam();
  Histogram histogram{
      .count = test_case.input_count,
      .sum = test_case.sum,
      .type = test_case.input_type,
  };
  auto buckets = test_case.classic_buckets;

  // Act
  histogram.convert_to_native(buckets);

  // Assert
  EXPECT_EQ(test_case.expected, histogram);
}

INSTANTIATE_TEST_SUITE_P(Empty,
                         ConvertClassicToNativeHistogramFixture,
                         testing::Values(ConvertCase{
                             .input_type = HistogramType::kInt,
                             .input_count = {.value = 0},
                             .classic_buckets = {},
                             .expected =
                                 {
                                     .count = {.value = 0},
                                     .positive_spans = {},
                                     .positive_buckets = {},
                                     .custom_values = {},
                                     .type = HistogramType::kInt,
                                     .schema = kCustomBucketsSchema,
                                 },
                         }));

INSTANTIATE_TEST_SUITE_P(SingleIntegerBucket,
                         ConvertClassicToNativeHistogramFixture,
                         testing::Values(ConvertCase{
                             .input_type = HistogramType::kInt,
                             .input_count = {.value = 0},
                             .sum = 1000.25,
                             .classic_buckets = {uint_bucket(0.5, 1000)},
                             .expected =
                                 {
                                     .count = {.value = 1000},
                                     .sum = 1000.25,
                                     .positive_spans = {{.offset = 0, .length = 1}},
                                     .positive_buckets = BareBones::Vector<HistogramBucketValue>{{.value = 1000}},
                                     .custom_values = BareBones::Vector<double>{0.5},
                                     .type = HistogramType::kInt,
                                     .schema = kCustomBucketsSchema,
                                 },
                         }));

INSTANTIATE_TEST_SUITE_P(SingleFloatBucket,
                         ConvertClassicToNativeHistogramFixture,
                         testing::Values(ConvertCase{
                             .input_type = HistogramType::kInt,
                             .input_count = {.value = 0},
                             .sum = 1000.25,
                             .classic_buckets = {float_bucket(0.5, 1337.42)},
                             .expected =
                                 {
                                     .count = {.float_value = 1337.42},
                                     .sum = 1000.25,
                                     .positive_spans = {{.offset = 0, .length = 1}},
                                     .positive_buckets = BareBones::Vector<HistogramBucketValue>{{.float_value = 1337.42}},
                                     .custom_values = BareBones::Vector<double>{0.5},
                                     .type = HistogramType::kFloat,
                                     .schema = kCustomBucketsSchema,
                                 },
                         }));

INSTANTIATE_TEST_SUITE_P(HappyCaseIntegerBucket,
                         ConvertClassicToNativeHistogramFixture,
                         testing::Values(ConvertCase{
                             .input_type = HistogramType::kInt,
                             .input_count = {.value = 1000},
                             .sum = 1000.25,
                             .classic_buckets =
                                 {
                                     uint_bucket(0.5, 50),
                                     uint_bucket(1.0, 950),
                                     uint_bucket(kInf, 1000),
                                 },
                             .expected =
                                 {
                                     .count = {.value = 1000},
                                     .sum = 1000.25,
                                     .positive_spans = {{.offset = 0, .length = 3}},
                                     .positive_buckets = BareBones::Vector<HistogramBucketValue>{{.value = 50}, {.value = 850}, {.value = -850}},
                                     .custom_values = BareBones::Vector<double>{0.5, 1.0},
                                     .type = HistogramType::kInt,
                                     .schema = kCustomBucketsSchema,
                                 },
                         }));

INSTANTIATE_TEST_SUITE_P(
    HappyCaseFloatBucket,
    ConvertClassicToNativeHistogramFixture,
    testing::Values(ConvertCase{
        .input_type = HistogramType::kFloat,
        .input_count = {.float_value = 1000},
        .sum = 1000.25,
        .classic_buckets =
            {
                float_bucket(0.5, 50),
                float_bucket(1.0, 950.5),
                float_bucket(kInf, 1000),
            },
        .expected =
            {
                .count = {.float_value = 1000},
                .sum = 1000.25,
                .positive_spans = {{.offset = 0, .length = 3}},
                .positive_buckets = BareBones::Vector<HistogramBucketValue>{{.float_value = 50}, {.float_value = 900.5}, {.float_value = 49.5}},
                .custom_values = BareBones::Vector<double>{0.5, 1.0},
                .type = HistogramType::kFloat,
                .schema = kCustomBucketsSchema,
            },
    }));

INSTANTIATE_TEST_SUITE_P(MixedOrder,
                         ConvertClassicToNativeHistogramFixture,
                         testing::Values(ConvertCase{
                             .input_type = HistogramType::kInt,
                             .input_count = {.value = 1000},
                             .sum = 1000.25,
                             .classic_buckets =
                                 {
                                     uint_bucket(0.5, 50),
                                     uint_bucket(kInf, 1000),
                                     uint_bucket(1.0, 950),
                                 },
                             .expected =
                                 {
                                     .count = {.value = 1000},
                                     .sum = 1000.25,
                                     .positive_spans = {{.offset = 0, .length = 3}},
                                     .positive_buckets = BareBones::Vector<HistogramBucketValue>{{.value = 50}, {.value = 850}, {.value = -850}},
                                     .custom_values = BareBones::Vector<double>{0.5, 1.0},
                                     .type = HistogramType::kInt,
                                     .schema = kCustomBucketsSchema,
                                 },
                         }));

INSTANTIATE_TEST_SUITE_P(MissingInf,
                         ConvertClassicToNativeHistogramFixture,
                         testing::Values(ConvertCase{
                             .input_type = HistogramType::kInt,
                             .input_count = {.value = 1000},
                             .sum = 1000.25,
                             .classic_buckets =
                                 {
                                     uint_bucket(0.5, 50),
                                     uint_bucket(1.0, 950),
                                 },
                             .expected =
                                 {
                                     .count = {.value = 1000},
                                     .sum = 1000.25,
                                     .positive_spans = {{.offset = 0, .length = 3}},
                                     .positive_buckets = BareBones::Vector<HistogramBucketValue>{{.value = 50}, {.value = 850}, {.value = -850}},
                                     .custom_values = BareBones::Vector<double>{0.5, 1.0},
                                     .type = HistogramType::kInt,
                                     .schema = kCustomBucketsSchema,
                                 },
                         }));

INSTANTIATE_TEST_SUITE_P(CountInference,
                         ConvertClassicToNativeHistogramFixture,
                         testing::Values(ConvertCase{
                             .input_type = HistogramType::kInt,
                             .input_count = {.value = 0},
                             .sum = 1000.25,
                             .classic_buckets =
                                 {
                                     uint_bucket(0.5, 50),
                                     uint_bucket(1.0, 950),
                                 },
                             .expected =
                                 {
                                     .count = {.value = 950},
                                     .sum = 1000.25,
                                     .positive_spans = {{.offset = 0, .length = 2}},
                                     .positive_buckets = BareBones::Vector<HistogramBucketValue>{{.value = 50}, {.value = 850}},
                                     .custom_values = BareBones::Vector<double>{0.5, 1.0},
                                     .type = HistogramType::kInt,
                                     .schema = kCustomBucketsSchema,
                                 },
                         }));

[[nodiscard]] BareBones::Vector<HistogramBucketValue> make_int_buckets(std::initializer_list<int64_t> buckets) {
  BareBones::Vector<HistogramBucketValue> result;
  result.reserve(buckets.size());
  for (const auto value : buckets) {
    result.emplace_back(HistogramBucketValue{.value = value});
  }
  return result;
}

[[nodiscard]] BareBones::Vector<HistogramBucketValue> make_float_buckets(std::initializer_list<double> buckets) {
  BareBones::Vector<HistogramBucketValue> result;
  result.reserve(buckets.size());
  for (const auto value : buckets) {
    result.emplace_back(HistogramBucketValue{.float_value = value});
  }
  return result;
}

PROMPP_ALWAYS_INLINE std::string format_test_case_name(std::string_view name) {
  std::string formatted_name(name.begin(), name.end());
  std::ranges::replace_if(formatted_name, [](auto ch) { return !std::isalnum(ch); }, '_');
  return formatted_name;
}

struct CompactCase {
  std::string_view name;
  int max_empty_buckets;
  Histogram input;
  Histogram expected;
};

class HistogramCompactIntFixture : public ::testing::TestWithParam<CompactCase> {};

TEST_P(HistogramCompactIntFixture, Test) {
  // Arrange
  const auto& test_case = GetParam();
  auto histogram = test_case.input;

  // Act
  histogram.compact_buckets(test_case.max_empty_buckets);
  auto histogram2 = histogram;
  histogram2.compact_buckets(test_case.max_empty_buckets);

  // Assert
  EXPECT_EQ(test_case.expected, histogram);
  EXPECT_EQ(test_case.expected, histogram2);
}

INSTANTIATE_TEST_SUITE_P(
    AllCases,
    HistogramCompactIntFixture,
    testing::Values(
        CompactCase{
            .name = "empty histogram",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {},
                    .negative_spans = {},
                    .positive_buckets = make_int_buckets({}),
                    .negative_buckets = make_int_buckets({}),
                    .type = HistogramType::kInt,
                },
            .expected =
                Histogram{
                    .positive_spans = {},
                    .negative_spans = {},
                    .positive_buckets = make_int_buckets({}),
                    .negative_buckets = make_int_buckets({}),
                    .type = HistogramType::kInt,
                },
        },
        CompactCase{
            .name = "nothing should happen",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 1}, {.offset = 2, .length = 3}},
                    .negative_spans = {{.offset = 3, .length = 2}, {.offset = 3, .length = 2}},
                    .positive_buckets = make_int_buckets({1, 3, -3, 42}),
                    .negative_buckets = make_int_buckets({5, 3, 123400, 1000}),
                    .type = HistogramType::kInt,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 1}, {.offset = 2, .length = 3}},
                    .negative_spans = {{.offset = 3, .length = 2}, {.offset = 3, .length = 2}},
                    .positive_buckets = make_int_buckets({1, 3, -3, 42}),
                    .negative_buckets = make_int_buckets({5, 3, 123400, 1000}),
                    .type = HistogramType::kInt,
                },
        },
        CompactCase{
            .name = "eliminate zero offsets",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 1}, {.offset = 0, .length = 3}, {.offset = 0, .length = 1}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 0, .length = 2}, {.offset = 2, .length = 1}, {.offset = 0, .length = 1}},
                    .positive_buckets = make_int_buckets({1, 3, -3, 42, 3}),
                    .negative_buckets = make_int_buckets({5, 3, 123400, 1000, 3, 4}),
                    .type = HistogramType::kInt,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 5}},
                    .negative_spans = {{.offset = 0, .length = 4}, {.offset = 2, .length = 2}},
                    .positive_buckets = make_int_buckets({1, 3, -3, 42, 3}),
                    .negative_buckets = make_int_buckets({5, 3, 123400, 1000, 3, 4}),
                    .type = HistogramType::kInt,
                },
        },
        CompactCase{
            .name = "eliminate zero length",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 2}, {.offset = 2, .length = 0}, {.offset = 3, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 0, .length = 0}, {.offset = 2, .length = 0}, {.offset = 1, .length = 4}},
                    .positive_buckets = make_int_buckets({1, 3, -3, 42, 3}),
                    .negative_buckets = make_int_buckets({5, 3, 123400, 1000, 3, 4}),
                    .type = HistogramType::kInt,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 2}, {.offset = 5, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 4}},
                    .positive_buckets = make_int_buckets({1, 3, -3, 42, 3}),
                    .negative_buckets = make_int_buckets({5, 3, 123400, 1000, 3, 4}),
                    .type = HistogramType::kInt,
                },
        },
        CompactCase{
            .name = "eliminate multiple zero length spans",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 2},
                                       {.offset = 2, .length = 0},
                                       {.offset = 2, .length = 0},
                                       {.offset = 2, .length = 0},
                                       {.offset = 3, .length = 3}},
                    .negative_spans = {},
                    .positive_buckets = make_int_buckets({1, 3, -3, 42, 3}),
                    .negative_buckets = make_int_buckets({}),
                    .type = HistogramType::kInt,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 2}, {.offset = 9, .length = 3}},
                    .negative_spans = {},
                    .positive_buckets = make_int_buckets({1, 3, -3, 42, 3}),
                    .negative_buckets = make_int_buckets({}),
                    .type = HistogramType::kInt,
                },
        },
        CompactCase{
            .name = "cut empty buckets at start or end",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 4}, {.offset = 5, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 5}},
                    .positive_buckets = make_int_buckets({0, 0, 1, 3, -3, 42, 3}),
                    .negative_buckets = make_int_buckets({5, 3, -4, -2, 3, 4, -9}),
                    .type = HistogramType::kInt,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 2}, {.offset = 5, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 4}},
                    .positive_buckets = make_int_buckets({1, 3, -3, 42, 3}),
                    .negative_buckets = make_int_buckets({5, 3, -4, -2, 3, 4}),
                    .type = HistogramType::kInt,
                },
        },
        CompactCase{
            .name = "cut empty buckets at start and end",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 4}, {.offset = 5, .length = 6}},
                    .negative_spans = {{.offset = -2, .length = 4}, {.offset = 3, .length = 5}},
                    .positive_buckets = make_int_buckets({0, 0, 1, 3, -3, 42, 3, -46, 0, 0}),
                    .negative_buckets = make_int_buckets({0, 0, 5, 3, -4, -2, 3, 4, -9}),
                    .type = HistogramType::kInt,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 2}, {.offset = 5, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 4}},
                    .positive_buckets = make_int_buckets({1, 3, -3, 42, 3}),
                    .negative_buckets = make_int_buckets({5, 3, -4, -2, 3, 4}),
                    .type = HistogramType::kInt,
                },
        },
        CompactCase{
            .name = "cut empty buckets at start or end of spans, even in the middle",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 6}, {.offset = 3, .length = 6}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 2, .length = 6}},
                    .positive_buckets = make_int_buckets({0, 0, 1, 3, -4, 0, 1, 42, 3, -46, 0, 0}),
                    .negative_buckets = make_int_buckets({5, 3, -8, 4, -2, 3, 4, -9}),
                    .type = HistogramType::kInt,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 2}, {.offset = 5, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 4}},
                    .positive_buckets = make_int_buckets({1, 3, -3, 42, 3}),
                    .negative_buckets = make_int_buckets({5, 3, -4, -2, 3, 4}),
                    .type = HistogramType::kInt,
                },
        },
        CompactCase{
            .name = "cut empty buckets at start or end but merge spans due to maxEmptyBuckets",
            .max_empty_buckets = 10,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 4}, {.offset = 5, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 5}},
                    .positive_buckets = make_int_buckets({0, 0, 1, 3, -3, 42, 3}),
                    .negative_buckets = make_int_buckets({5, 3, -4, -2, 3, 4, -9}),
                    .type = HistogramType::kInt,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 10}},
                    .negative_spans = {{.offset = 0, .length = 9}},
                    .positive_buckets = make_int_buckets({1, 3, -4, 0, 0, 0, 0, 1, 42, 3}),
                    .negative_buckets = make_int_buckets({5, 3, -8, 0, 0, 4, -2, 3, 4}),
                    .type = HistogramType::kInt,
                },
        },
        CompactCase{
            .name = "cut empty buckets from the middle of a span",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 6}, {.offset = 3, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 5}},
                    .positive_buckets = make_int_buckets({0, 0, 1, -1, 0, 3, -2, 42, 3}),
                    .negative_buckets = make_int_buckets({5, 3, -4, -2, -2, 3, 4}),
                    .type = HistogramType::kInt,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 1}, {.offset = 2, .length = 1}, {.offset = 3, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 2}, {.offset = 1, .length = 2}},
                    .positive_buckets = make_int_buckets({1, 2, -2, 42, 3}),
                    .negative_buckets = make_int_buckets({5, 3, -4, -2, 1, 4}),
                    .type = HistogramType::kInt,
                },
        },
        CompactCase{
            .name = "cut out a span containing only empty buckets",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 3}, {.offset = 2, .length = 2}, {.offset = 3, .length = 4}},
                    .negative_spans = {},
                    .positive_buckets = make_int_buckets({0, 0, 1, -1, 0, 3, -2, 42, 3}),
                    .negative_buckets = make_int_buckets({}),
                    .type = HistogramType::kInt,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 1}, {.offset = 7, .length = 4}},
                    .negative_spans = {},
                    .positive_buckets = make_int_buckets({1, 2, -2, 42, 3}),
                    .negative_buckets = make_int_buckets({}),
                    .type = HistogramType::kInt,
                },
        },
        CompactCase{
            .name = "cut empty buckets from the middle of a span, avoiding some due to maxEmptyBuckets",
            .max_empty_buckets = 1,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 6}, {.offset = 3, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 5}},
                    .positive_buckets = make_int_buckets({0, 0, 1, -1, 0, 3, -2, 42, 3}),
                    .negative_buckets = make_int_buckets({5, 3, -4, -2, -2, 3, 4}),
                    .type = HistogramType::kInt,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 1}, {.offset = 2, .length = 1}, {.offset = 3, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 5}},
                    .positive_buckets = make_int_buckets({1, 2, -2, 42, 3}),
                    .negative_buckets = make_int_buckets({5, 3, -4, -2, -2, 3, 4}),
                    .type = HistogramType::kInt,
                },
        },
        CompactCase{
            .name = "avoiding all cutting of empty buckets from the middle of a chunk due to maxEmptyBuckets",
            .max_empty_buckets = 2,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 6}, {.offset = 3, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 5}},
                    .positive_buckets = make_int_buckets({0, 0, 1, -1, 0, 3, -2, 42, 3}),
                    .negative_buckets = make_int_buckets({5, 3, -4, -2, -2, 3, 4}),
                    .type = HistogramType::kInt,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 4}, {.offset = 3, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 5}},
                    .positive_buckets = make_int_buckets({1, -1, 0, 3, -2, 42, 3}),
                    .negative_buckets = make_int_buckets({5, 3, -4, -2, -2, 3, 4}),
                    .type = HistogramType::kInt,
                },
        },
        CompactCase{
            .name = "everything merged into one span due to maxEmptyBuckets",
            .max_empty_buckets = 3,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 6}, {.offset = 3, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 5}},
                    .positive_buckets = make_int_buckets({0, 0, 1, -1, 0, 3, -2, 42, 3}),
                    .negative_buckets = make_int_buckets({5, 3, -4, -2, -2, 3, 4}),
                    .type = HistogramType::kInt,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 10}},
                    .negative_spans = {{.offset = 0, .length = 10}},
                    .positive_buckets = make_int_buckets({1, -1, 0, 3, -3, 0, 0, 1, 42, 3}),
                    .negative_buckets = make_int_buckets({5, 3, -8, 0, 0, 4, -2, -2, 3, 4}),
                    .type = HistogramType::kInt,
                },
        },
        CompactCase{
            .name = "only empty buckets and maxEmptyBuckets greater zero",
            .max_empty_buckets = 3,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 6}, {.offset = 3, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 7}},
                    .positive_buckets = make_int_buckets({0, 0, 0, 0, 0, 0, 0, 0, 0}),
                    .negative_buckets = make_int_buckets({0, 0, 0, 0, 0, 0, 0}),
                    .type = HistogramType::kInt,
                },
            .expected =
                Histogram{
                    .positive_spans = {},
                    .negative_spans = {},
                    .positive_buckets = make_int_buckets({}),
                    .negative_buckets = make_int_buckets({}),
                    .type = HistogramType::kInt,
                },
        },
        CompactCase{
            .name = "multiple spans of only empty buckets",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -10, .length = 2}, {.offset = 2, .length = 1}, {.offset = 3, .length = 3}},
                    .negative_spans = {{.offset = -10, .length = 2}, {.offset = 2, .length = 1}, {.offset = 3, .length = 3}},
                    .positive_buckets = make_int_buckets({0, 0, 0, 0, 2, 3}),
                    .negative_buckets = make_int_buckets({2, 3, -5, 0, 0, 0}),
                    .type = HistogramType::kInt,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -1, .length = 2}},
                    .negative_spans = {{.offset = -10, .length = 2}},
                    .positive_buckets = make_int_buckets({2, 3}),
                    .negative_buckets = make_int_buckets({2, 3}),
                    .type = HistogramType::kInt,
                },
        },
        CompactCase{
            .name = "nothing should happen with custom buckets",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 1}, {.offset = 2, .length = 3}},
                    .negative_spans = {},
                    .positive_buckets = make_int_buckets({1, 3, -3, 42}),
                    .negative_buckets = make_int_buckets({}),
                    .custom_values = BareBones::Vector<double>{5, 10, 15},
                    .type = HistogramType::kInt,
                    .schema = kCustomBucketsSchema,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 1}, {.offset = 2, .length = 3}},
                    .negative_spans = {},
                    .positive_buckets = make_int_buckets({1, 3, -3, 42}),
                    .negative_buckets = make_int_buckets({}),
                    .custom_values = BareBones::Vector<double>{5, 10, 15},
                    .type = HistogramType::kInt,
                    .schema = kCustomBucketsSchema,
                },
        },
        CompactCase{
            .name = "eliminate zero offsets with custom buckets",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 1}, {.offset = 0, .length = 3}, {.offset = 0, .length = 1}},
                    .negative_spans = {},
                    .positive_buckets = make_int_buckets({1, 3, -3, 42, 3}),
                    .negative_buckets = make_int_buckets({}),
                    .custom_values = BareBones::Vector<double>{5, 10, 15, 20},
                    .type = HistogramType::kInt,
                    .schema = kCustomBucketsSchema,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 5}},
                    .negative_spans = {},
                    .positive_buckets = make_int_buckets({1, 3, -3, 42, 3}),
                    .negative_buckets = make_int_buckets({}),
                    .custom_values = BareBones::Vector<double>{5, 10, 15, 20},
                    .type = HistogramType::kInt,
                    .schema = kCustomBucketsSchema,
                },
        },
        CompactCase{
            .name = "eliminate zero length with custom buckets",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 2}, {.offset = 2, .length = 0}, {.offset = 3, .length = 3}},
                    .negative_spans = {},
                    .positive_buckets = make_int_buckets({1, 3, -3, 42, 3}),
                    .negative_buckets = make_int_buckets({}),
                    .custom_values = BareBones::Vector<double>{5, 10, 15, 20},
                    .type = HistogramType::kInt,
                    .schema = kCustomBucketsSchema,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 2}, {.offset = 5, .length = 3}},
                    .negative_spans = {},
                    .positive_buckets = make_int_buckets({1, 3, -3, 42, 3}),
                    .negative_buckets = make_int_buckets({}),
                    .custom_values = BareBones::Vector<double>{5, 10, 15, 20},
                    .type = HistogramType::kInt,
                    .schema = kCustomBucketsSchema,
                },
        },
        CompactCase{
            .name = "all zero-length spans with non-empty buckets",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = 0, .length = 0}, {.offset = 2, .length = 0}},
                    .negative_spans = {{.offset = 1, .length = 0}},
                    .positive_buckets = make_int_buckets({1, 3}),
                    .negative_buckets = make_int_buckets({2}),
                    .type = HistogramType::kInt,
                },
            .expected =
                Histogram{
                    .positive_spans = {},
                    .negative_spans = {},
                    .positive_buckets = make_int_buckets({}),
                    .negative_buckets = make_int_buckets({}),
                    .type = HistogramType::kInt,
                },
        },
        CompactCase{
            .name = "more buckets than spans account for",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = 0, .length = 1}, {.offset = 2, .length = 1}},
                    .negative_spans = {{.offset = 0, .length = 1}},
                    .positive_buckets = make_int_buckets({1, 2, 3, 4}),
                    .negative_buckets = make_int_buckets({5, 6}),
                    .type = HistogramType::kInt,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = 0, .length = 1}, {.offset = 2, .length = 1}},
                    .negative_spans = {{.offset = 0, .length = 1}},
                    .positive_buckets = make_int_buckets({1, 2, 3, 4}),
                    .negative_buckets = make_int_buckets({5, 6}),
                    .type = HistogramType::kInt,
                },
        },
        CompactCase{
            .name = "eliminate multiple zero length spans with custom buckets",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 2},
                                       {.offset = 2, .length = 0},
                                       {.offset = 2, .length = 0},
                                       {.offset = 2, .length = 0},
                                       {.offset = 3, .length = 3}},
                    .negative_spans = {},
                    .positive_buckets = make_int_buckets({1, 3, -3, 42, 3}),
                    .negative_buckets = make_int_buckets({}),
                    .custom_values = BareBones::Vector<double>{5, 10, 15, 20},
                    .type = HistogramType::kInt,
                    .schema = kCustomBucketsSchema,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 2}, {.offset = 9, .length = 3}},
                    .negative_spans = {},
                    .positive_buckets = make_int_buckets({1, 3, -3, 42, 3}),
                    .negative_buckets = make_int_buckets({}),
                    .custom_values = BareBones::Vector<double>{5, 10, 15, 20},
                    .type = HistogramType::kInt,
                    .schema = kCustomBucketsSchema,
                },
        },
        CompactCase{
            .name = "cut empty buckets at start or end of spans, even in the middle, with custom buckets",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 6}, {.offset = 3, .length = 6}},
                    .negative_spans = {},
                    .positive_buckets = make_int_buckets({0, 0, 1, 3, -4, 0, 1, 42, 3, -46, 0, 0}),
                    .negative_buckets = make_int_buckets({}),
                    .custom_values = BareBones::Vector<double>{5, 10, 15, 20},
                    .type = HistogramType::kInt,
                    .schema = kCustomBucketsSchema,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 2}, {.offset = 5, .length = 3}},
                    .negative_spans = {},
                    .positive_buckets = make_int_buckets({1, 3, -3, 42, 3}),
                    .negative_buckets = make_int_buckets({}),
                    .custom_values = BareBones::Vector<double>{5, 10, 15, 20},
                    .type = HistogramType::kInt,
                    .schema = kCustomBucketsSchema,
                },
        }),
    [](const testing::TestParamInfo<CompactCase>& info) { return format_test_case_name(info.param.name); });

class HistogramCompactFloatFixture : public ::testing::TestWithParam<CompactCase> {};

TEST_P(HistogramCompactFloatFixture, Test) {
  // Arrange
  const auto& test_case = GetParam();
  auto histogram = test_case.input;

  // Act
  histogram.compact_buckets(test_case.max_empty_buckets);
  auto histogram2 = histogram;
  histogram2.compact_buckets(test_case.max_empty_buckets);

  // Assert
  EXPECT_EQ(test_case.expected, histogram);
  EXPECT_EQ(test_case.expected, histogram2);
}

INSTANTIATE_TEST_SUITE_P(
    AllCases,
    HistogramCompactFloatFixture,
    testing::Values(
        CompactCase{
            .name = "empty histogram",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({}),
                    .negative_buckets = make_float_buckets({}),
                    .type = HistogramType::kFloat,
                },
            .expected =
                Histogram{
                    .positive_spans = {},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({}),
                    .negative_buckets = make_float_buckets({}),
                    .type = HistogramType::kFloat,
                },
        },
        CompactCase{
            .name = "nothing should happen",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 1}, {.offset = 2, .length = 3}},
                    .negative_spans = {{.offset = 3, .length = 2}, {.offset = 3, .length = 2}},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1}),
                    .negative_buckets = make_float_buckets({3.1, 3, 123400, 1000}),
                    .type = HistogramType::kFloat,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 1}, {.offset = 2, .length = 3}},
                    .negative_spans = {{.offset = 3, .length = 2}, {.offset = 3, .length = 2}},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1}),
                    .negative_buckets = make_float_buckets({3.1, 3, 123400, 1000}),
                    .type = HistogramType::kFloat,
                },
        },
        CompactCase{
            .name = "eliminate zero offsets",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 1}, {.offset = 0, .length = 3}, {.offset = 0, .length = 1}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 0, .length = 2}, {.offset = 2, .length = 1}, {.offset = 0, .length = 1}},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({3.1, 3, 123400, 1000, 3, 4}),
                    .type = HistogramType::kFloat,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 5}},
                    .negative_spans = {{.offset = 0, .length = 4}, {.offset = 2, .length = 2}},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({3.1, 3, 123400, 1000, 3, 4}),
                    .type = HistogramType::kFloat,
                },
        },
        CompactCase{
            .name = "eliminate zero length",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 2}, {.offset = 2, .length = 0}, {.offset = 3, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 0, .length = 0}, {.offset = 2, .length = 0}, {.offset = 1, .length = 4}},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({3.1, 3, 123400, 1000, 3, 4}),
                    .type = HistogramType::kFloat,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 2}, {.offset = 5, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 4}},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({3.1, 3, 123400, 1000, 3, 4}),
                    .type = HistogramType::kFloat,
                },
        },
        CompactCase{
            .name = "eliminate multiple zero length spans",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 2},
                                       {.offset = 2, .length = 0},
                                       {.offset = 2, .length = 0},
                                       {.offset = 2, .length = 0},
                                       {.offset = 3, .length = 3}},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({}),
                    .type = HistogramType::kFloat,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 2}, {.offset = 9, .length = 3}},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({}),
                    .type = HistogramType::kFloat,
                },
        },
        CompactCase{
            .name = "cut empty buckets at start or end",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 4}, {.offset = 5, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 5}},
                    .positive_buckets = make_float_buckets({0, 0, 1, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({3.1, 3, 123400, 1000, 3, 4, 0}),
                    .type = HistogramType::kFloat,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 2}, {.offset = 5, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 4}},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({3.1, 3, 123400, 1000, 3, 4}),
                    .type = HistogramType::kFloat,
                },
        },
        CompactCase{
            .name = "cut empty buckets at start and end",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 4}, {.offset = 5, .length = 6}},
                    .negative_spans = {{.offset = -2, .length = 4}, {.offset = 3, .length = 5}},
                    .positive_buckets = make_float_buckets({0, 0, 1, 3.3, 4.2, 0.1, 3.3, 0, 0, 0}),
                    .negative_buckets = make_float_buckets({0, 0, 3.1, 3, 123400, 1000, 3, 4, 0}),
                    .type = HistogramType::kFloat,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 2}, {.offset = 5, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 4}},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({3.1, 3, 123400, 1000, 3, 4}),
                    .type = HistogramType::kFloat,
                },
        },
        CompactCase{
            .name = "cut empty buckets in the middle",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = 5, .length = 4}},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({1, 3, 0, 2}),
                    .negative_buckets = make_float_buckets({}),
                    .type = HistogramType::kFloat,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = 5, .length = 2}, {.offset = 1, .length = 1}},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({1, 3, 2}),
                    .negative_buckets = make_float_buckets({}),
                    .type = HistogramType::kFloat,
                },
        },
        CompactCase{
            .name = "cut empty buckets at start or end of spans, even in the middle",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 6}, {.offset = 3, .length = 6}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 2, .length = 6}},
                    .positive_buckets = make_float_buckets({0, 0, 1, 3.3, 0, 0, 4.2, 0.1, 3.3, 0, 0, 0}),
                    .negative_buckets = make_float_buckets({3.1, 3, 0, 123400, 1000, 3, 4, 0}),
                    .type = HistogramType::kFloat,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 2}, {.offset = 5, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 4}},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({3.1, 3, 123400, 1000, 3, 4}),
                    .type = HistogramType::kFloat,
                },
        },
        CompactCase{
            .name = "cut empty buckets at start and end - also merge spans due to maxEmptyBuckets",
            .max_empty_buckets = 10,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 4}, {.offset = 5, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 5}},
                    .positive_buckets = make_float_buckets({0, 0, 1, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({3.1, 3, 123400, 1000, 3, 4, 0}),
                    .type = HistogramType::kFloat,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 10}},
                    .negative_spans = {{.offset = 0, .length = 9}},
                    .positive_buckets = make_float_buckets({1, 3.3, 0, 0, 0, 0, 0, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({3.1, 3, 0, 0, 0, 123400, 1000, 3, 4}),
                    .type = HistogramType::kFloat,
                },
        },
        CompactCase{
            .name = "cut empty buckets from the middle of a span",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 6}, {.offset = 3, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 5}},
                    .positive_buckets = make_float_buckets({0, 0, 1, 0, 0, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({3.1, 3, 123400, 1000, 0, 3, 4}),
                    .type = HistogramType::kFloat,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 1}, {.offset = 2, .length = 1}, {.offset = 3, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 2}, {.offset = 1, .length = 2}},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({3.1, 3, 123400, 1000, 3, 4}),
                    .type = HistogramType::kFloat,
                },
        },
        CompactCase{
            .name = "cut out a span containing only empty buckets",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 3}, {.offset = 2, .length = 2}, {.offset = 3, .length = 4}},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({0, 0, 1, 0, 0, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({}),
                    .type = HistogramType::kFloat,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 1}, {.offset = 7, .length = 4}},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({}),
                    .type = HistogramType::kFloat,
                },
        },
        CompactCase{
            .name = "cut empty buckets from the middle of a span, avoiding none due to maxEmptyBuckets",
            .max_empty_buckets = 1,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 4}},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({1, 0, 0, 3.3}),
                    .negative_buckets = make_float_buckets({}),
                    .type = HistogramType::kFloat,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 1}, {.offset = 2, .length = 1}},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({1, 3.3}),
                    .negative_buckets = make_float_buckets({}),
                    .type = HistogramType::kFloat,
                },
        },
        CompactCase{
            .name = "cut empty buckets and merge spans due to maxEmptyBuckets",
            .max_empty_buckets = 1,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 4}, {.offset = 3, .length = 1}},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({1, 0, 0, 3.3, 4.2}),
                    .negative_buckets = make_float_buckets({}),
                    .type = HistogramType::kFloat,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 1}, {.offset = 2, .length = 1}, {.offset = 3, .length = 1}},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2}),
                    .negative_buckets = make_float_buckets({}),
                    .type = HistogramType::kFloat,
                },
        },
        CompactCase{
            .name = "cut empty buckets from the middle of a span, avoiding some due to maxEmptyBuckets",
            .max_empty_buckets = 1,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 6}, {.offset = 3, .length = 3}, {.offset = 10, .length = 2}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 5}},
                    .positive_buckets = make_float_buckets({0, 0, 1, 0, 0, 3.3, 4.2, 0.1, 3.3, 2, 3}),
                    .negative_buckets = make_float_buckets({3.1, 3, 123400, 1000, 0, 3, 4}),
                    .type = HistogramType::kFloat,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 1}, {.offset = 2, .length = 1}, {.offset = 3, .length = 3}, {.offset = 10, .length = 2}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 5}},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1, 3.3, 2, 3}),
                    .negative_buckets = make_float_buckets({3.1, 3, 123400, 1000, 0, 3, 4}),
                    .type = HistogramType::kFloat,
                },
        },
        CompactCase{
            .name = "avoiding all cutting of empty buckets from the middle of a chunk due to maxEmptyBuckets",
            .max_empty_buckets = 2,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 6}, {.offset = 3, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 5}},
                    .positive_buckets = make_float_buckets({0, 0, 1, 0, 0, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({3.1, 3, 123400, 1000, 0, 3, 4}),
                    .type = HistogramType::kFloat,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 4}, {.offset = 3, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 5}},
                    .positive_buckets = make_float_buckets({1, 0, 0, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({3.1, 3, 123400, 1000, 0, 3, 4}),
                    .type = HistogramType::kFloat,
                },
        },
        CompactCase{
            .name = "everything merged into one span due to maxEmptyBuckets",
            .max_empty_buckets = 3,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 6}, {.offset = 3, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 2}, {.offset = 3, .length = 5}},
                    .positive_buckets = make_float_buckets({0, 0, 1, 0, 0, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({3.1, 3, 123400, 1000, 0, 3, 4}),
                    .type = HistogramType::kFloat,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -2, .length = 10}},
                    .negative_spans = {{.offset = 0, .length = 10}},
                    .positive_buckets = make_float_buckets({1, 0, 0, 3.3, 0, 0, 0, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({3.1, 3, 0, 0, 0, 123400, 1000, 0, 3, 4}),
                    .type = HistogramType::kFloat,
                },
        },
        CompactCase{
            .name = "only empty buckets and maxEmptyBuckets greater zero",
            .max_empty_buckets = 3,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -4, .length = 6}, {.offset = 3, .length = 3}},
                    .negative_spans = {{.offset = 0, .length = 7}},
                    .positive_buckets = make_float_buckets({0, 0, 0, 0, 0, 0, 0, 0, 0}),
                    .negative_buckets = make_float_buckets({0, 0, 0, 0, 0, 0, 0}),
                    .type = HistogramType::kFloat,
                },
            .expected =
                Histogram{
                    .positive_spans = {},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({}),
                    .negative_buckets = make_float_buckets({}),
                    .type = HistogramType::kFloat,
                },
        },
        CompactCase{
            .name = "multiple spans of only empty buckets",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = -10, .length = 2}, {.offset = 2, .length = 1}, {.offset = 3, .length = 3}},
                    .negative_spans = {{.offset = -10, .length = 2}, {.offset = 2, .length = 1}, {.offset = 3, .length = 3}},
                    .positive_buckets = make_float_buckets({0, 0, 0, 0, 2, 3}),
                    .negative_buckets = make_float_buckets({2, 3, 0, 0, 0, 0}),
                    .type = HistogramType::kFloat,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = -1, .length = 2}},
                    .negative_spans = {{.offset = -10, .length = 2}},
                    .positive_buckets = make_float_buckets({2, 3}),
                    .negative_buckets = make_float_buckets({2, 3}),
                    .type = HistogramType::kFloat,
                },
        },
        CompactCase{
            .name = "nothing should happen with custom buckets",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = 0, .length = 1}, {.offset = 2, .length = 3}},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1}),
                    .negative_buckets = make_float_buckets({}),
                    .custom_values = BareBones::Vector<double>{1, 2, 3},
                    .type = HistogramType::kFloat,
                    .schema = kCustomBucketsSchema,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = 0, .length = 1}, {.offset = 2, .length = 3}},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1}),
                    .negative_buckets = make_float_buckets({}),
                    .custom_values = BareBones::Vector<double>{1, 2, 3},
                    .type = HistogramType::kFloat,
                    .schema = kCustomBucketsSchema,
                },
        },
        CompactCase{
            .name = "eliminate zero offsets with custom buckets",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = 0, .length = 1}, {.offset = 0, .length = 3}, {.offset = 0, .length = 1}},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({}),
                    .custom_values = BareBones::Vector<double>{1, 2, 3, 4},
                    .type = HistogramType::kFloat,
                    .schema = kCustomBucketsSchema,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = 0, .length = 5}},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({}),
                    .custom_values = BareBones::Vector<double>{1, 2, 3, 4},
                    .type = HistogramType::kFloat,
                    .schema = kCustomBucketsSchema,
                },
        },
        CompactCase{
            .name = "eliminate multiple zero length spans with custom buckets",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = 0, .length = 2},
                                       {.offset = 2, .length = 0},
                                       {.offset = 2, .length = 0},
                                       {.offset = 2, .length = 0},
                                       {.offset = 3, .length = 3}},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({}),
                    .custom_values = BareBones::Vector<double>{1, 2, 3, 4},
                    .type = HistogramType::kFloat,
                    .schema = kCustomBucketsSchema,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = 0, .length = 2}, {.offset = 9, .length = 3}},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({}),
                    .custom_values = BareBones::Vector<double>{1, 2, 3, 4},
                    .type = HistogramType::kFloat,
                    .schema = kCustomBucketsSchema,
                },
        },
        CompactCase{
            .name = "cut empty buckets at start and end with custom buckets",
            .max_empty_buckets = 0,
            .input =
                Histogram{
                    .positive_spans = {{.offset = 0, .length = 4}, {.offset = 5, .length = 6}},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({0, 0, 1, 3.3, 4.2, 0.1, 3.3, 0, 0, 0}),
                    .negative_buckets = make_float_buckets({}),
                    .custom_values = BareBones::Vector<double>{1, 2, 3, 4, 5, 6, 7, 8, 9},
                    .type = HistogramType::kFloat,
                    .schema = kCustomBucketsSchema,
                },
            .expected =
                Histogram{
                    .positive_spans = {{.offset = 2, .length = 2}, {.offset = 5, .length = 3}},
                    .negative_spans = {},
                    .positive_buckets = make_float_buckets({1, 3.3, 4.2, 0.1, 3.3}),
                    .negative_buckets = make_float_buckets({}),
                    .custom_values = BareBones::Vector<double>{1, 2, 3, 4, 5, 6, 7, 8, 9},
                    .type = HistogramType::kFloat,
                    .schema = kCustomBucketsSchema,
                },
        }),
    [](const testing::TestParamInfo<CompactCase>& info) { return format_test_case_name(info.param.name); });

}  // namespace
