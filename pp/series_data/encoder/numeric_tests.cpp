#include <gtest/gtest.h>

#include <cfenv>
#include <cmath>
#include <limits>

#include "series_data/encoder/numeric.h"

namespace {

static_assert(series_data::encoder::value::is_int(1.0));
static_assert(!series_data::encoder::value::is_int(1.5));
static_assert(series_data::encoder::value::is_int(-0x1p63));
static_assert(!series_data::encoder::value::is_int(0x1p63));

struct NumericIntegerCase {
  const char* name;
  double value;
  bool expected;
};

class NumericIntegerRangeFixture : public testing::TestWithParam<NumericIntegerCase> {};

TEST_P(NumericIntegerRangeFixture, ChecksEligibilityWithoutInvalidConversion) {
  // Arrange
  const auto& test_case = GetParam();

  // Act
  const bool actual = series_data::encoder::value::is_int(test_case.value);

  // Assert
  EXPECT_EQ(test_case.expected, actual);
}

INSTANTIATE_TEST_SUITE_P(Boundaries,
                         NumericIntegerRangeFixture,
                         testing::Values(NumericIntegerCase{.name = "MinInt64", .value = -0x1p63, .expected = true},
                                         NumericIntegerCase{.name = "AboveMinInt64", .value = std::nextafter(-0x1p63, 0.0), .expected = true},
                                         NumericIntegerCase{.name = "BelowMinInt64", .value = std::nextafter(-0x1p63, -INFINITY), .expected = false},
                                         NumericIntegerCase{.name = "BelowInt64UpperBound", .value = std::nextafter(0x1p63, 0.0), .expected = true},
                                         NumericIntegerCase{.name = "Int64UpperBound", .value = 0x1p63, .expected = false},
                                         NumericIntegerCase{.name = "MaxDouble", .value = std::numeric_limits<double>::max(), .expected = false},
                                         NumericIntegerCase{.name = "LowestDouble", .value = -std::numeric_limits<double>::max(), .expected = false},
                                         NumericIntegerCase{.name = "PositiveInfinity", .value = INFINITY, .expected = false},
                                         NumericIntegerCase{.name = "NegativeInfinity", .value = -INFINITY, .expected = false},
                                         NumericIntegerCase{.name = "Nan", .value = std::numeric_limits<double>::quiet_NaN(), .expected = false},
                                         NumericIntegerCase{.name = "Zero", .value = 0.0, .expected = true},
                                         NumericIntegerCase{.name = "NegativeZero", .value = -0.0, .expected = true},
                                         NumericIntegerCase{.name = "PositiveFraction", .value = 1.5, .expected = false},
                                         NumericIntegerCase{.name = "NegativeFraction", .value = -1.5, .expected = false}),
                         [](const testing::TestParamInfo<NumericIntegerCase>& info) { return info.param.name; });

struct NumericFloatCase {
  const char* name;
  double value;
  bool expected;
};

class NumericFloatRangeFixture : public testing::TestWithParam<NumericFloatCase> {};

TEST_P(NumericFloatRangeFixture, ChecksEligibilityWithoutNarrowingOverflow) {
  // Arrange
  const auto& test_case = GetParam();
  std::feclearexcept(FE_OVERFLOW);

  // Act
  const bool actual = series_data::encoder::value::is_float(test_case.value);
  const int overflow = std::fetestexcept(FE_OVERFLOW);

  // Assert
  EXPECT_EQ(test_case.expected, actual);
  EXPECT_EQ(0, overflow);
}

INSTANTIATE_TEST_SUITE_P(Boundaries,
                         NumericFloatRangeFixture,
                         testing::Values(NumericFloatCase{.name = "MaxDouble", .value = std::numeric_limits<double>::max(), .expected = false},
                                         NumericFloatCase{.name = "LowestDouble", .value = -std::numeric_limits<double>::max(), .expected = false},
                                         NumericFloatCase{.name = "MaxFloat", .value = std::numeric_limits<float>::max(), .expected = true},
                                         NumericFloatCase{.name = "LowestFloat", .value = std::numeric_limits<float>::lowest(), .expected = true},
                                         NumericFloatCase{.name = "AboveMaxFloat",
                                                          .value = std::nextafter(static_cast<double>(std::numeric_limits<float>::max()), INFINITY),
                                                          .expected = false},
                                         NumericFloatCase{.name = "SmallestFloat", .value = std::numeric_limits<float>::denorm_min(), .expected = true},
                                         NumericFloatCase{.name = "PositiveInfinity", .value = INFINITY, .expected = true},
                                         NumericFloatCase{.name = "NegativeInfinity", .value = -INFINITY, .expected = true},
                                         NumericFloatCase{.name = "QuietNan", .value = std::numeric_limits<double>::quiet_NaN(), .expected = true},
                                         NumericFloatCase{.name = "StaleNan", .value = std::bit_cast<double>(0x7ff0000000000002ULL), .expected = false},
                                         NumericFloatCase{.name = "Zero", .value = 0.0, .expected = true},
                                         NumericFloatCase{.name = "NegativeZero", .value = -0.0, .expected = true},
                                         NumericFloatCase{.name = "ExactFraction", .value = 1.5, .expected = true},
                                         NumericFloatCase{.name = "InexactFraction", .value = 1.1, .expected = false}),
                         [](const testing::TestParamInfo<NumericFloatCase>& info) { return info.param.name; });

}  // namespace
