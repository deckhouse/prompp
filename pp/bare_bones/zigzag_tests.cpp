#include <limits>
#include <random>

#include <gtest/gtest.h>

#include "bare_bones/zigzag.h"

namespace {

const size_t NUM_VALUES = 10000;

struct ZigZag : public testing::Test {};

TEST_F(ZigZag, EncodeDecodeInt64T) {
  int64_t val;
  std::mt19937 gen32(testing::UnitTest::GetInstance()->random_seed());

  for (size_t i = 0; i < NUM_VALUES; ++i) {
    if (i % 2 == 0) {
      val = 0 - static_cast<int64_t>(gen32()) / 2;
    } else {
      val = static_cast<int64_t>(gen32()) / 2;
    }

    const uint64_t enc_val = BareBones::Encoding::ZigZag::encode(val);
    int64_t dec_val = BareBones::Encoding::ZigZag::decode(enc_val);

    EXPECT_EQ(val, dec_val);
  }
}

TEST_F(ZigZag, EncodeDecodeInt32T) {
  int32_t val;

  std::mt19937 gen32(testing::UnitTest::GetInstance()->random_seed());

  for (size_t i = 0; i < NUM_VALUES; ++i) {
    if (i % 2 == 0) {
      val = 0 - static_cast<int32_t>(gen32()) / 2;
    } else {
      val = static_cast<int32_t>(gen32()) / 2;
    }

    const uint32_t enc_val = BareBones::Encoding::ZigZag::encode(val);
    int32_t dec_val = BareBones::Encoding::ZigZag::decode(enc_val);

    EXPECT_EQ(val, dec_val);
  }
}

struct ZigZagDecodeCase64 {
  const char* name;
  uint64_t encoded;
  int64_t expected;
};

class ZigZagDecode64Fixture : public testing::TestWithParam<ZigZagDecodeCase64> {};

TEST_P(ZigZagDecode64Fixture, DecodesFullSignedRange) {
  // Arrange
  const auto& param = GetParam();

  // Act
  const auto decoded = BareBones::Encoding::ZigZag::decode(param.encoded);

  // Assert
  EXPECT_EQ(param.expected, decoded);
}

INSTANTIATE_TEST_SUITE_P(
    BoundaryValues,
    ZigZagDecode64Fixture,
    testing::Values(ZigZagDecodeCase64{.name = "Zero", .encoded = 0, .expected = 0},
                    ZigZagDecodeCase64{.name = "NegativeOne", .encoded = 1, .expected = -1},
                    ZigZagDecodeCase64{.name = "PositiveOne", .encoded = 2, .expected = 1},
                    ZigZagDecodeCase64{.name = "NegativeTwo", .encoded = 3, .expected = -2},
                    ZigZagDecodeCase64{.name = "Maximum", .encoded = std::numeric_limits<uint64_t>::max() - 1, .expected = std::numeric_limits<int64_t>::max()},
                    ZigZagDecodeCase64{.name = "Minimum", .encoded = std::numeric_limits<uint64_t>::max(), .expected = std::numeric_limits<int64_t>::min()}),
    [](const auto& info) { return info.param.name; });

struct ZigZagDecodeCase32 {
  const char* name;
  uint32_t encoded;
  int32_t expected;
};

class ZigZagDecode32Fixture : public testing::TestWithParam<ZigZagDecodeCase32> {};

TEST_P(ZigZagDecode32Fixture, DecodesFullSignedRange) {
  // Arrange
  const auto& param = GetParam();

  // Act
  const auto decoded = BareBones::Encoding::ZigZag::decode(param.encoded);

  // Assert
  EXPECT_EQ(param.expected, decoded);
}

INSTANTIATE_TEST_SUITE_P(
    BoundaryValues,
    ZigZagDecode32Fixture,
    testing::Values(ZigZagDecodeCase32{.name = "Zero", .encoded = 0, .expected = 0},
                    ZigZagDecodeCase32{.name = "NegativeOne", .encoded = 1, .expected = -1},
                    ZigZagDecodeCase32{.name = "PositiveOne", .encoded = 2, .expected = 1},
                    ZigZagDecodeCase32{.name = "NegativeTwo", .encoded = 3, .expected = -2},
                    ZigZagDecodeCase32{.name = "Maximum", .encoded = std::numeric_limits<uint32_t>::max() - 1, .expected = std::numeric_limits<int32_t>::max()},
                    ZigZagDecodeCase32{.name = "Minimum", .encoded = std::numeric_limits<uint32_t>::max(), .expected = std::numeric_limits<int32_t>::min()}),
    [](const auto& info) { return info.param.name; });
}  // namespace
