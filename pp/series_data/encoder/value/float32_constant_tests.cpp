#include <gtest/gtest.h>

#include <cstdint>
#include <limits>

#include "bare_bones/gorilla.h"
#include "series_data/common.h"
#include "series_data/encoder/value/float32_constant.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using series_data::encoder::value::Float32ConstantEncoder;

struct CanBeEncodedCase {
  double value;
  bool expected;
};

class Float32ConstantEncoderCanBeEncodedFixture : public testing::TestWithParam<CanBeEncodedCase> {};

TEST_P(Float32ConstantEncoderCanBeEncodedFixture, Test) {
  // Arrange
  auto& test_case = GetParam();

  // Act
  auto result = Float32ConstantEncoder::can_be_encoded(test_case.value);

  // Assert
  EXPECT_EQ(test_case.expected, result);
}

INSTANTIATE_TEST_SUITE_P(Tests,
                         Float32ConstantEncoderCanBeEncodedFixture,
                         testing::Values(CanBeEncodedCase{.value = STALE_NAN, .expected = false},
                                         CanBeEncodedCase{.value = 0.0, .expected = true},
                                         CanBeEncodedCase{.value = 1.0, .expected = true},
                                         CanBeEncodedCase{.value = 1.1, .expected = false},
                                         CanBeEncodedCase{.value = -1.0, .expected = true},
                                         CanBeEncodedCase{.value = std::numeric_limits<uint32_t>::max(), .expected = false},
                                         CanBeEncodedCase{.value = 128.625, .expected = true},
                                         CanBeEncodedCase{.value = std::numeric_limits<float>::max(), .expected = true},
                                         CanBeEncodedCase{.value = std::numeric_limits<float>::min(), .expected = true}));

struct EncodeCase {
  double initial_value;
  double value;
  bool expected;
};

class Float32ConstantEncoderEncodeFixture : public testing::TestWithParam<EncodeCase> {
 protected:
  Float32ConstantEncoder encoder_{GetParam().initial_value};
  series_data::EncodingState state{.encoding_type = series_data::EncodingType::kFloat32Constant, .has_last_stalenan = false};
};

TEST_P(Float32ConstantEncoderEncodeFixture, Encode) {
  // Arrange

  // Act
  const auto result = encoder_.encode(state, GetParam().value);

  // Assert
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(Test,
                         Float32ConstantEncoderEncodeFixture,
                         testing::Values(EncodeCase{.initial_value = -1.0, .value = 0.0, .expected = false},
                                         EncodeCase{.initial_value = -1.0, .value = 1.0, .expected = false},
                                         EncodeCase{.initial_value = -1.0, .value = STALE_NAN, .expected = true},
                                         EncodeCase{.initial_value = -1.0, .value = -1.0, .expected = true}));

class Float32ConstantEncoderEncodIsActualFixture : public testing::TestWithParam<EncodeCase> {
 protected:
  Float32ConstantEncoder encoder_{GetParam().initial_value};
  series_data::EncodingState state{.encoding_type = series_data::EncodingType::kFloat32Constant, .has_last_stalenan = false};
};

TEST_P(Float32ConstantEncoderEncodIsActualFixture, IsActual) {
  // Arrange

  // Act
  const auto result = encoder_.is_actual(state, GetParam().value);

  // Assert
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(Test,
                         Float32ConstantEncoderEncodIsActualFixture,
                         testing::Values(EncodeCase{.initial_value = -1.0, .value = 0.0, .expected = false},
                                         EncodeCase{.initial_value = -1.0, .value = 1.0, .expected = false},
                                         EncodeCase{.initial_value = -1.0, .value = STALE_NAN, .expected = false},
                                         EncodeCase{.initial_value = -1.0, .value = -1.0, .expected = true}));

class Float32ConstantEncoderEncodIsActualStalenanFixture : public Float32ConstantEncoderEncodIsActualFixture {};

TEST_P(Float32ConstantEncoderEncodIsActualStalenanFixture, IsActual) {
  // Arrange
  const auto stalenan_encoded = encoder_.encode(state, STALE_NAN);

  // Act
  const auto result = encoder_.is_actual(state, GetParam().value);

  // Assert
  EXPECT_TRUE(stalenan_encoded);
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(Test,
                         Float32ConstantEncoderEncodIsActualStalenanFixture,
                         testing::Values(EncodeCase{.initial_value = -1.0, .value = STALE_NAN, .expected = true},
                                         EncodeCase{.initial_value = 1.0, .value = STALE_NAN, .expected = true},
                                         EncodeCase{.initial_value = 0.0, .value = STALE_NAN, .expected = true},
                                         EncodeCase{.initial_value = 0.0, .value = 0.0, .expected = false}));

}  // namespace