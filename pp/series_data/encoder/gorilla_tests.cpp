#include <gtest/gtest.h>

#include <cstdint>

#include "bare_bones/gorilla.h"
#include "series_data/common.h"
#include "series_data/encoder/gorilla.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using GorillaEncoder = series_data::encoder::GorillaEncoder<BareBones::DefaultReallocator>;

struct IsActualCase {
  BareBones::Vector<double> values;
  double value;
  bool expected;
};

class GorillaEncoderIsActualFixture : public testing::TestWithParam<IsActualCase> {
 protected:
  GorillaEncoder encode(const BareBones::Vector<double>& values) {
    GorillaEncoder encoder(ts++, values[0]);
    for (size_t i = 1; i < values.size(); ++i) {
      encoder.encode(state, ts++, values[i]);
    }
    return encoder;
  }
  series_data::EncodingState state{.encoding_type = series_data::EncodingType::kGorilla, .has_last_stalenan = false};
  int64_t ts = 0;
};

TEST_P(GorillaEncoderIsActualFixture, Test) {
  // Arrange
  const auto encoder = encode(GetParam().values);

  // Act
  const auto result = encoder.is_actual(state, GetParam().value);

  // Assert
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(TwoPoints,
                         GorillaEncoderIsActualFixture,
                         testing::Values(IsActualCase{.values = {1.1, 2.0}, .value = 1.0, .expected = false},
                                         IsActualCase{.values = {1.1, 2.0}, .value = 2.0, .expected = true},
                                         IsActualCase{.values = {1.1, STALE_NAN}, .value = 1.0, .expected = false},
                                         IsActualCase{.values = {STALE_NAN, 1.0}, .value = 2.0, .expected = false}));

INSTANTIATE_TEST_SUITE_P(ThreePoints,
                         GorillaEncoderIsActualFixture,
                         testing::Values(IsActualCase{.values = {1.0, 2.1, 3.0}, .value = 2.0, .expected = false},
                                         IsActualCase{.values = {1.0, 2.1, 3.0}, .value = 3.0, .expected = true},
                                         IsActualCase{.values = {1.0, STALE_NAN, 2.0}, .value = 2.0, .expected = true},
                                         IsActualCase{.values = {1.0, STALE_NAN, 2.0}, .value = STALE_NAN, .expected = false}));

INSTANTIATE_TEST_SUITE_P(FourPoints,
                         GorillaEncoderIsActualFixture,
                         testing::Values(IsActualCase{.values = {1.0, 2.0, 3.1, 4.0}, .value = 2.0, .expected = false},
                                         IsActualCase{.values = {1.0, 2.0, 3.1, 4.0}, .value = 4.0, .expected = true},
                                         IsActualCase{.values = {1.0, 2.0, STALE_NAN, 4.1}, .value = 4.1, .expected = true},
                                         IsActualCase{.values = {1.0, 2.0, 3.1, STALE_NAN}, .value = STALE_NAN, .expected = true},
                                         IsActualCase{.values = {1.0, 2.0, 3.1, STALE_NAN}, .value = 3.0, .expected = false}));

INSTANTIATE_TEST_SUITE_P(NonIntegerValue,
                         GorillaEncoderIsActualFixture,
                         testing::Values(IsActualCase{.values = {-1.0, 0.0}, .value = -1.1, .expected = false},
                                         IsActualCase{.values = {-1.0, 12.5}, .value = 12.5, .expected = true}));

}  // namespace
