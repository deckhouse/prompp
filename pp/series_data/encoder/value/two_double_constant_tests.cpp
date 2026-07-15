#include <gtest/gtest.h>

#include <cstdint>
#include <limits>

#include "bare_bones/gorilla.h"
#include "series_data/common.h"
#include "series_data/encoder/value/two_double_constant.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using series_data::encoder::value::TwoDoubleConstantEncoder;

struct EncodeCase {
  double value1;
  double value2;
  uint8_t value1_count;
  double value;
  bool expected;
};

class TwoDoubleConstantEncoderEncodeFixture : public testing::TestWithParam<EncodeCase> {
 protected:
  TwoDoubleConstantEncoder encoder_{GetParam().value1, GetParam().value2, GetParam().value1_count};
  series_data::EncodingState state{.encoding_type = series_data::EncodingType::kTwoDoubleConstant, .has_last_stalenan = false};
};

TEST_P(TwoDoubleConstantEncoderEncodeFixture, Encode) {
  // Arrange

  // Act
  const auto result = encoder_.encode(state, GetParam().value);

  // Assert
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(Test,
                         TwoDoubleConstantEncoderEncodeFixture,
                         testing::Values(EncodeCase{.value1 = -1.0, .value2 = 1.0, .value1_count = 1, .value = 0.0, .expected = false},
                                         EncodeCase{.value1 = 0.0, .value2 = 1.0, .value1_count = 1, .value = 1.0, .expected = true},
                                         EncodeCase{.value1 = std::numeric_limits<double>::min(),
                                                    .value2 = std::numeric_limits<double>::max(),
                                                    .value1_count = 255,
                                                    .value = std::numeric_limits<double>::max(),
                                                    .expected = true}));

class TwoDoubleConstantEncoderEncodIsActualFixture : public testing::TestWithParam<EncodeCase> {
 protected:
  TwoDoubleConstantEncoder encoder_{GetParam().value1, GetParam().value2, GetParam().value1_count};
  series_data::EncodingState state{series_data::EncodingType::kDoubleConstant, false};
};

TEST_P(TwoDoubleConstantEncoderEncodIsActualFixture, IsActual) {
  // Arrange

  // Act
  const auto result = encoder_.is_actual(state, GetParam().value);

  // Assert
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(Test,
                         TwoDoubleConstantEncoderEncodIsActualFixture,
                         testing::Values(EncodeCase{.value1 = -1.0, .value2 = 1.0, .value1_count = 1, .value = 0.0, .expected = false},
                                         EncodeCase{.value1 = -1.0, .value2 = 1.0, .value1_count = 1, .value = 1.0, .expected = true}));

class TwoDoubleConstantEncoderEncodIsActualStalenanFixture : public TwoDoubleConstantEncoderEncodIsActualFixture {};

TEST_P(TwoDoubleConstantEncoderEncodIsActualStalenanFixture, IsActual) {
  // Arrange
  const auto stalenan_encoded = encoder_.encode(state, STALE_NAN);

  // Act
  const auto result = encoder_.is_actual(state, GetParam().value);

  // Assert
  EXPECT_TRUE(stalenan_encoded);
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(Test,
                         TwoDoubleConstantEncoderEncodIsActualStalenanFixture,
                         testing::Values(EncodeCase{.value1 = -1.0, .value2 = 1.0, .value1_count = 1, .value = STALE_NAN, .expected = true},
                                         EncodeCase{.value1 = -1.0, .value2 = 1.0, .value1_count = 1, .value = 1.0, .expected = false}));

}  // namespace