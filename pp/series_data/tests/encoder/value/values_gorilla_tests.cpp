#include <gtest/gtest.h>

#include "bare_bones/gorilla.h"
#include "series_data/common.h"
#include "series_data/encoder/value/values_gorilla.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using series_data::encoder::value::ValuesGorillaEncoder;

struct IsActualCase {
  BareBones::Vector<double> values;
  double value;
  bool expected;
};

class ValuesGorillaEncoderIsActualFixture : public testing::TestWithParam<IsActualCase> {
 protected:
  ValuesGorillaEncoder encode(const BareBones::Vector<double>& values) {
    ValuesGorillaEncoder encoder(values[0], 1);
    for (size_t i = 1; i < values.size(); ++i) {
      encoder.encode(state, values[i]);
    }
    return encoder;
  }
  series_data::EncodingState state{.encoding_type = series_data::EncodingType::kValuesGorilla, .has_last_stalenan = false};
};

TEST_P(ValuesGorillaEncoderIsActualFixture, Test) {
  // Arrange
  auto encoder = encode(GetParam().values);

  // Act
  auto result = encoder.is_actual(state, GetParam().value);

  // Assert
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(TwoPoints,
                         ValuesGorillaEncoderIsActualFixture,
                         testing::Values(IsActualCase{.values = {1.1, 2.0}, .value = 1.0, .expected = false},
                                         IsActualCase{.values = {1.1, 2.0}, .value = 2.0, .expected = true},
                                         IsActualCase{.values = {1.1, STALE_NAN}, .value = 1.0, .expected = false},
                                         IsActualCase{.values = {STALE_NAN, 1.0}, .value = 2.0, .expected = false}));

INSTANTIATE_TEST_SUITE_P(ThreePoints,
                         ValuesGorillaEncoderIsActualFixture,
                         testing::Values(IsActualCase{.values = {1.0, 2.1, 3.0}, .value = 2.0, .expected = false},
                                         IsActualCase{.values = {1.0, 2.1, 3.0}, .value = 3.0, .expected = true},
                                         IsActualCase{.values = {1.0, STALE_NAN, 2.0}, .value = 2.0, .expected = true},
                                         IsActualCase{.values = {1.0, STALE_NAN, 2.0}, .value = STALE_NAN, .expected = false}));

INSTANTIATE_TEST_SUITE_P(FourPoints,
                         ValuesGorillaEncoderIsActualFixture,
                         testing::Values(IsActualCase{.values = {1.0, 2.0, 3.1, 4.0}, .value = 2.0, .expected = false},
                                         IsActualCase{.values = {1.0, 2.0, 3.1, 4.0}, .value = 4.0, .expected = true},
                                         IsActualCase{.values = {1.0, 2.0, STALE_NAN, 4.1}, .value = 4.1, .expected = true},
                                         IsActualCase{.values = {1.0, 2.0, 3.1, STALE_NAN}, .value = STALE_NAN, .expected = true},
                                         IsActualCase{.values = {1.0, 2.0, 3.1, STALE_NAN}, .value = 3.0, .expected = false}));

INSTANTIATE_TEST_SUITE_P(NonIntegerValue,
                         ValuesGorillaEncoderIsActualFixture,
                         testing::Values(IsActualCase{.values = {-1.0, 0.0}, .value = -1.1, .expected = false},
                                         IsActualCase{.values = {-1.0, 12.5}, .value = 12.5, .expected = true}));

}  // namespace
