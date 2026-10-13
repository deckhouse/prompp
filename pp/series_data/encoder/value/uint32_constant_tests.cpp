#include <gtest/gtest.h>

#include <cstdint>
#include <limits>

#include "bare_bones/gorilla.h"
#include "series_data/common.h"
#include "series_data/encoder/value/uint32_constant.h"

namespace {

using BareBones::Encoding::Gorilla::STALE_NAN;
using series_data::encoder::value::Uint32ConstantEncoder;

struct CanBeEncodedCase {
  double value;
  bool expected;
};

class Uint32ConstantEncoderCanBeEncodedFixture : public testing::TestWithParam<CanBeEncodedCase> {};

TEST_P(Uint32ConstantEncoderCanBeEncodedFixture, Test) {
  // Arrange
  auto& test_case = GetParam();

  // Act
  auto result = Uint32ConstantEncoder::can_be_encoded(test_case.value);

  // Assert
  EXPECT_EQ(test_case.expected, result);
}

INSTANTIATE_TEST_SUITE_P(Tests,
                         Uint32ConstantEncoderCanBeEncodedFixture,
                         testing::Values(CanBeEncodedCase{.value = STALE_NAN, .expected = false},
                                         CanBeEncodedCase{.value = 0.0, .expected = true},
                                         CanBeEncodedCase{.value = 1.0, .expected = true},
                                         CanBeEncodedCase{.value = 1.1, .expected = false},
                                         CanBeEncodedCase{.value = -1.0, .expected = false},
                                         CanBeEncodedCase{.value = std::numeric_limits<uint32_t>::max(), .expected = true},
                                         CanBeEncodedCase{.value = std::numeric_limits<uint32_t>::max() + 1.0, .expected = false}));

struct EncodeCase {
  double initial_value;
  double value;
  bool expected;
};

class Uint32ConstantEncoderEncodeFixture : public testing::TestWithParam<EncodeCase> {
 protected:
  Uint32ConstantEncoder encoder_{GetParam().initial_value};
  series_data::EncodingState state{.encoding_type = series_data::EncodingType::kUint32Constant, .has_last_stalenan = false};
};

TEST_P(Uint32ConstantEncoderEncodeFixture, Encode) {
  // Arrange

  // Act
  auto result = encoder_.encode(state, GetParam().value);

  // Assert
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(
    Test,
    Uint32ConstantEncoderEncodeFixture,
    testing::Values(EncodeCase{.initial_value = 0.0, .value = 0.0, .expected = true},
                    EncodeCase{.initial_value = 0.0, .value = 1.0, .expected = false},
                    EncodeCase{.initial_value = 0.0, .value = STALE_NAN, .expected = true},
                    EncodeCase{.initial_value = 0.0, .value = -1.0, .expected = false},
                    EncodeCase{.initial_value = std::numeric_limits<uint32_t>::max(), .value = std::numeric_limits<uint32_t>::max() + 1.0, .expected = false},
                    EncodeCase{.initial_value = std::numeric_limits<uint32_t>::max(), .value = std::numeric_limits<uint32_t>::max(), .expected = true}));

class Uint32ConstantEncoderEncodIsActualFixture : public testing::TestWithParam<EncodeCase> {
 protected:
  Uint32ConstantEncoder encoder_{GetParam().initial_value};
  series_data::EncodingState state{.encoding_type = series_data::EncodingType::kUint32Constant, .has_last_stalenan = false};
};

TEST_P(Uint32ConstantEncoderEncodIsActualFixture, IsActual) {
  // Arrange

  // Act
  auto result = encoder_.is_actual(state, GetParam().value);

  // Assert
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(
    Test,
    Uint32ConstantEncoderEncodIsActualFixture,
    testing::Values(EncodeCase{.initial_value = 0.0, .value = 0.0, .expected = true},
                    EncodeCase{.initial_value = 0.0, .value = 1.0, .expected = false},
                    EncodeCase{.initial_value = 0.0, .value = STALE_NAN, .expected = false},
                    EncodeCase{.initial_value = 0.0, .value = -1.0, .expected = false},
                    EncodeCase{.initial_value = std::numeric_limits<uint32_t>::max(), .value = std::numeric_limits<uint32_t>::max() + 1.0, .expected = false},
                    EncodeCase{.initial_value = std::numeric_limits<uint32_t>::max(), .value = std::numeric_limits<uint32_t>::max(), .expected = true}));

class Uint32ConstantEncoderEncodIsActualStalenanFixture : public Uint32ConstantEncoderEncodIsActualFixture {};

TEST_P(Uint32ConstantEncoderEncodIsActualStalenanFixture, IsActual) {
  // Arrange
  const auto stalenan_encoded = encoder_.encode(state, STALE_NAN);

  // Act
  const auto result = encoder_.is_actual(state, GetParam().value);

  // Assert
  EXPECT_TRUE(stalenan_encoded);
  EXPECT_EQ(GetParam().expected, result);
}

INSTANTIATE_TEST_SUITE_P(Test,
                         Uint32ConstantEncoderEncodIsActualStalenanFixture,
                         testing::Values(EncodeCase{.initial_value = 0.0, .value = STALE_NAN, .expected = true},
                                         EncodeCase{.initial_value = 1.0, .value = STALE_NAN, .expected = true},
                                         EncodeCase{.initial_value = std::numeric_limits<uint32_t>::max(), .value = STALE_NAN, .expected = true},
                                         EncodeCase{.initial_value = 1.0, .value = 1.0, .expected = false}));

}  // namespace