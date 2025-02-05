#include <gtest/gtest.h>

#include <cstdint>
#include <limits>

#include "bare_bones/gorilla.h"
#include "series_data/common.h"
#include "series_data/encoder/value/double_constant.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using series_data::encoder::value::DoubleConstantEncoder;

struct EncodeCase {
  double initial_value;
  double value;
  bool expected;
};

class DoubleConstantEncoderEncodeFixture : public testing::TestWithParam<EncodeCase> {
 protected:
  DoubleConstantEncoder encoder_{GetParam().initial_value};
  series_data::EncodingState state{.encoding_type = series_data::EncodingType::kDoubleConstant, .has_last_stalenan = false};
};

TEST_P(DoubleConstantEncoderEncodeFixture, Encode) {
  // Arrange

  // Act
  const auto result = encoder_.encode(state, GetParam().value);

  // Assert
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(
    Test,
    DoubleConstantEncoderEncodeFixture,
    testing::Values(EncodeCase{.initial_value = -1.0, .value = 0.0, .expected = false},
                    EncodeCase{.initial_value = -1.0, .value = 1.0, .expected = false},
                    EncodeCase{.initial_value = -1.0, .value = STALE_NAN, .expected = true},
                    EncodeCase{.initial_value = -1.0, .value = -1.0, .expected = true},
                    EncodeCase{.initial_value = STALE_NAN, .value = STALE_NAN, .expected = true},
                    EncodeCase{.initial_value = 1.1, .value = 1.0, .expected = false},
                    EncodeCase{.initial_value = 1.1, .value = 1.1, .expected = true},
                    EncodeCase{.initial_value = std::numeric_limits<double>::max(), .value = std::numeric_limits<double>::max(), .expected = true},
                    EncodeCase{.initial_value = std::numeric_limits<double>::min(), .value = std::numeric_limits<double>::min(), .expected = true}));

class DoubleConstantEncoderEncodIsActualFixture : public testing::TestWithParam<EncodeCase> {
 protected:
  DoubleConstantEncoder encoder_{GetParam().initial_value};
  series_data::EncodingState state{.encoding_type = series_data::EncodingType::kDoubleConstant, .has_last_stalenan = false};
};

TEST_P(DoubleConstantEncoderEncodIsActualFixture, IsActual) {
  // Arrange

  // Act
  const auto result = encoder_.is_actual(state, GetParam().value);

  // Assert
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(Test,
                         DoubleConstantEncoderEncodIsActualFixture,
                         testing::Values(EncodeCase{.initial_value = 1.1, .value = 0.0, .expected = false},
                                         EncodeCase{.initial_value = 1.1, .value = 1.0, .expected = false},
                                         EncodeCase{.initial_value = 1.1, .value = STALE_NAN, .expected = false},
                                         EncodeCase{.initial_value = 1.1, .value = 1.1, .expected = true}));

class DoubleConstantEncoderEncodIsActualStalenanFixture : public DoubleConstantEncoderEncodIsActualFixture {};

TEST_P(DoubleConstantEncoderEncodIsActualStalenanFixture, IsActual) {
  // Arrange
  const auto stalenan_encoded = encoder_.encode(state, STALE_NAN);

  // Act
  const auto result = encoder_.is_actual(state, GetParam().value);

  // Assert
  EXPECT_TRUE(stalenan_encoded);
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(Test,
                         DoubleConstantEncoderEncodIsActualStalenanFixture,
                         testing::Values(EncodeCase{.initial_value = -1.1, .value = STALE_NAN, .expected = true},
                                         EncodeCase{.initial_value = 1.1, .value = STALE_NAN, .expected = true},
                                         EncodeCase{.initial_value = 0.1, .value = STALE_NAN, .expected = true},
                                         EncodeCase{.initial_value = 0.1, .value = 0.1, .expected = false}));

}  // namespace