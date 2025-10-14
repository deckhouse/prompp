#include <gtest/gtest.h>

#include "bare_bones/varint.h"

namespace {

using BareBones::Encoding::VarInt;

struct VariantTestCase {
  std::variant<int8_t, uint8_t, int16_t, uint16_t, int32_t, uint32_t, int64_t, uint64_t> value;
  size_t expected_length;
};

class VarintFixture : public testing::TestWithParam<VariantTestCase> {
 protected:
  std::array<uint8_t, VarInt::kMaxVarIntLength<uint64_t>> buffer_{};
  BareBones::BitSequenceReader reader_{(buffer_.data()), BareBones::Bit::to_bits(buffer_.size())};
};

TEST_P(VarintFixture, WriteAndRead) {
  // Arrange

  // Act
  const auto bytes_written = std::visit([&](auto v) { return VarInt::write(buffer_.data(), v); }, GetParam().value);
  const auto decoded = std::visit([&]<typename T>([[maybe_unused]] T v) { return static_cast<uint64_t>(VarInt::read<T>(reader_)); }, GetParam().value);

  // Assert
  ASSERT_EQ(bytes_written, GetParam().expected_length);
  ASSERT_EQ(bytes_written, std::visit([&](auto v) { return VarInt::length(v); }, GetParam().value));
  EXPECT_EQ(decoded, std::visit([&](auto v) { return static_cast<uint64_t>(v); }, GetParam().value));
}

INSTANTIATE_TEST_SUITE_P(TestUnsigned,
                         VarintFixture,
                         testing::Values(VariantTestCase{.value = uint8_t{0}, .expected_length = 1},
                                         VariantTestCase{.value = uint8_t{69}, .expected_length = 1},
                                         VariantTestCase{.value = uint8_t{(1ULL << 7) - 1}, .expected_length = 1},
                                         VariantTestCase{.value = uint8_t{1ULL << 7}, .expected_length = 2},
                                         VariantTestCase{.value = uint8_t{175}, .expected_length = 2},
                                         VariantTestCase{.value = uint8_t{std::numeric_limits<uint8_t>::max()}, .expected_length = 2},

                                         VariantTestCase{.value = uint16_t{0}, .expected_length = 1},
                                         VariantTestCase{.value = uint16_t{10}, .expected_length = 1},
                                         VariantTestCase{.value = uint16_t{(1ULL << 7) - 1}, .expected_length = 1},
                                         VariantTestCase{.value = uint16_t{1ULL << 7}, .expected_length = 2},
                                         VariantTestCase{.value = uint16_t{199}, .expected_length = 2},
                                         VariantTestCase{.value = uint16_t{std::numeric_limits<uint8_t>::max()}, .expected_length = 2},
                                         VariantTestCase{.value = uint16_t{12345}, .expected_length = 2},
                                         VariantTestCase{.value = uint16_t{(1ULL << 14) - 1}, .expected_length = 2},
                                         VariantTestCase{.value = uint16_t{1ULL << 14}, .expected_length = 3},
                                         VariantTestCase{.value = uint16_t{32323}, .expected_length = 3},
                                         VariantTestCase{.value = uint16_t{std::numeric_limits<uint16_t>::max()}, .expected_length = 3},

                                         VariantTestCase{.value = uint32_t{0}, .expected_length = 1},
                                         VariantTestCase{.value = uint32_t{10}, .expected_length = 1},
                                         VariantTestCase{.value = uint32_t{(1ULL << 7) - 1}, .expected_length = 1},
                                         VariantTestCase{.value = uint32_t{1ULL << 7}, .expected_length = 2},
                                         VariantTestCase{.value = uint32_t{199}, .expected_length = 2},
                                         VariantTestCase{.value = uint32_t{std::numeric_limits<uint8_t>::max()}, .expected_length = 2},
                                         VariantTestCase{.value = uint32_t{12345}, .expected_length = 2},
                                         VariantTestCase{.value = uint32_t{(1ULL << 14) - 1}, .expected_length = 2},
                                         VariantTestCase{.value = uint32_t{1ULL << 14}, .expected_length = 3},
                                         VariantTestCase{.value = uint32_t{32323}, .expected_length = 3},
                                         VariantTestCase{.value = uint32_t{std::numeric_limits<uint16_t>::max()}, .expected_length = 3},
                                         VariantTestCase{.value = uint32_t{123456}, .expected_length = 3},
                                         VariantTestCase{.value = uint32_t{1488322}, .expected_length = 3},
                                         VariantTestCase{.value = uint32_t{(1ULL << 21) - 1}, .expected_length = 3},
                                         VariantTestCase{.value = uint32_t{1ULL << 21}, .expected_length = 4},
                                         VariantTestCase{.value = uint32_t{76543210}, .expected_length = 4},
                                         VariantTestCase{.value = uint32_t{123456789}, .expected_length = 4},
                                         VariantTestCase{.value = uint32_t{(1ULL << 28) - 1}, .expected_length = 4},
                                         VariantTestCase{.value = uint32_t{1ULL << 28}, .expected_length = 5},
                                         VariantTestCase{.value = uint32_t{268435456}, .expected_length = 5},
                                         VariantTestCase{.value = uint32_t{3790521346}, .expected_length = 5},
                                         VariantTestCase{.value = uint32_t{std::numeric_limits<uint32_t>::max()}, .expected_length = 5},

                                         VariantTestCase{.value = uint64_t{0ULL}, .expected_length = 1},
                                         VariantTestCase{.value = uint64_t{10ULL}, .expected_length = 1},
                                         VariantTestCase{.value = uint64_t{(1ULL << 7) - 1}, .expected_length = 1},
                                         VariantTestCase{.value = uint64_t{1ULL << 7}, .expected_length = 2},
                                         VariantTestCase{.value = uint64_t{199ULL}, .expected_length = 2},
                                         VariantTestCase{.value = uint64_t{std::numeric_limits<uint8_t>::max()}, .expected_length = 2},
                                         VariantTestCase{.value = uint64_t{12345ULL}, .expected_length = 2},
                                         VariantTestCase{.value = uint64_t{(1ULL << 14) - 1}, .expected_length = 2},
                                         VariantTestCase{.value = uint64_t{1ULL << 14}, .expected_length = 3},
                                         VariantTestCase{.value = uint64_t{32323ULL}, .expected_length = 3},
                                         VariantTestCase{.value = uint64_t{std::numeric_limits<uint16_t>::max()}, .expected_length = 3},
                                         VariantTestCase{.value = uint64_t{123456ULL}, .expected_length = 3},
                                         VariantTestCase{.value = uint64_t{1488322ULL}, .expected_length = 3},
                                         VariantTestCase{.value = uint64_t{(1ULL << 21) - 1}, .expected_length = 3},
                                         VariantTestCase{.value = uint64_t{1ULL << 21}, .expected_length = 4},
                                         VariantTestCase{.value = uint64_t{76543210ULL}, .expected_length = 4},
                                         VariantTestCase{.value = uint64_t{123456789ULL}, .expected_length = 4},
                                         VariantTestCase{.value = uint64_t{(1ULL << 28) - 1}, .expected_length = 4},
                                         VariantTestCase{.value = uint64_t{1ULL << 28}, .expected_length = 5},
                                         VariantTestCase{.value = uint64_t{268435456ULL}, .expected_length = 5},
                                         VariantTestCase{.value = uint64_t{3790521346ULL}, .expected_length = 5},
                                         VariantTestCase{.value = uint64_t{std::numeric_limits<uint32_t>::max()}, .expected_length = 5},
                                         VariantTestCase{.value = uint64_t{4294967295ULL}, .expected_length = 5},
                                         VariantTestCase{.value = uint64_t{(1ULL << 35) - 1}, .expected_length = 5},
                                         VariantTestCase{.value = uint64_t{1ULL << 35}, .expected_length = 6},
                                         VariantTestCase{.value = uint64_t{(1ULL << 42) - 1}, .expected_length = 6},
                                         VariantTestCase{.value = uint64_t{1ULL << 42}, .expected_length = 7},
                                         VariantTestCase{.value = uint64_t{(1ULL << 49) - 1}, .expected_length = 7},
                                         VariantTestCase{.value = uint64_t{1ULL << 49}, .expected_length = 8},
                                         VariantTestCase{.value = uint64_t{(1ULL << 56) - 1}, .expected_length = 8},
                                         VariantTestCase{.value = uint64_t{1ULL << 56}, .expected_length = 9},
                                         VariantTestCase{.value = uint64_t{(1ULL << 63) - 1}, .expected_length = 9},
                                         VariantTestCase{.value = uint64_t{1ULL << 63}, .expected_length = 10},
                                         VariantTestCase{.value = uint64_t{std::numeric_limits<uint64_t>::max()}, .expected_length = 10}));

INSTANTIATE_TEST_SUITE_P(TestSigned,
                         VarintFixture,
                         testing::Values(VariantTestCase{.value = int8_t{0}, .expected_length = 1},
                                         VariantTestCase{.value = int8_t{1}, .expected_length = 1},
                                         VariantTestCase{.value = int8_t{-1}, .expected_length = 1},
                                         VariantTestCase{.value = int8_t{63}, .expected_length = 1},
                                         VariantTestCase{.value = int8_t{-64}, .expected_length = 1},
                                         VariantTestCase{.value = int8_t{64}, .expected_length = 2},
                                         VariantTestCase{.value = int8_t{-65}, .expected_length = 2},
                                         VariantTestCase{.value = std::numeric_limits<int8_t>::max(), .expected_length = 2},
                                         VariantTestCase{.value = std::numeric_limits<int8_t>::min(), .expected_length = 2},

                                         VariantTestCase{.value = int16_t{0}, .expected_length = 1},
                                         VariantTestCase{.value = int16_t{1}, .expected_length = 1},
                                         VariantTestCase{.value = int16_t{-1}, .expected_length = 1},
                                         VariantTestCase{.value = int16_t{63}, .expected_length = 1},
                                         VariantTestCase{.value = int16_t{-64}, .expected_length = 1},
                                         VariantTestCase{.value = int16_t{64}, .expected_length = 2},
                                         VariantTestCase{.value = int16_t{-65}, .expected_length = 2},
                                         VariantTestCase{.value = int16_t{8191}, .expected_length = 2},
                                         VariantTestCase{.value = int16_t{-8192}, .expected_length = 2},
                                         VariantTestCase{.value = int16_t{8192}, .expected_length = 3},
                                         VariantTestCase{.value = int16_t{-8193}, .expected_length = 3},
                                         VariantTestCase{.value = std::numeric_limits<int16_t>::max(), .expected_length = 3},
                                         VariantTestCase{.value = std::numeric_limits<int16_t>::min(), .expected_length = 3},

                                         VariantTestCase{.value = int32_t{0}, .expected_length = 1},
                                         VariantTestCase{.value = int32_t{-1}, .expected_length = 1},
                                         VariantTestCase{.value = int32_t{1}, .expected_length = 1},
                                         VariantTestCase{.value = int32_t{63}, .expected_length = 1},
                                         VariantTestCase{.value = int32_t{-64}, .expected_length = 1},
                                         VariantTestCase{.value = int32_t{8192}, .expected_length = 3},
                                         VariantTestCase{.value = int32_t{-8193}, .expected_length = 3},
                                         VariantTestCase{.value = int32_t{1LL << 14}, .expected_length = 3},
                                         VariantTestCase{.value = int32_t{-(1LL << 14)}, .expected_length = 3},
                                         VariantTestCase{.value = int32_t{1LL << 21}, .expected_length = 4},
                                         VariantTestCase{.value = int32_t{-(1LL << 21)}, .expected_length = 4},
                                         VariantTestCase{.value = int32_t{1LL << 28}, .expected_length = 5},
                                         VariantTestCase{.value = int32_t{-(1LL << 28)}, .expected_length = 5},
                                         VariantTestCase{.value = std::numeric_limits<int32_t>::max(), .expected_length = 5},
                                         VariantTestCase{.value = std::numeric_limits<int32_t>::min(), .expected_length = 5},

                                         VariantTestCase{.value = int64_t{0}, .expected_length = 1},
                                         VariantTestCase{.value = int64_t{-1}, .expected_length = 1},
                                         VariantTestCase{.value = int64_t{1}, .expected_length = 1},
                                         VariantTestCase{.value = int64_t{63}, .expected_length = 1},
                                         VariantTestCase{.value = int64_t{-64}, .expected_length = 1},
                                         VariantTestCase{.value = int64_t{64}, .expected_length = 2},
                                         VariantTestCase{.value = int64_t{-65}, .expected_length = 2},
                                         VariantTestCase{.value = int64_t{8191}, .expected_length = 2},
                                         VariantTestCase{.value = int64_t{-8192}, .expected_length = 2},
                                         VariantTestCase{.value = int64_t{1LL << 14}, .expected_length = 3},
                                         VariantTestCase{.value = int64_t{-(1LL << 14)}, .expected_length = 3},
                                         VariantTestCase{.value = int64_t{1LL << 21}, .expected_length = 4},
                                         VariantTestCase{.value = int64_t{-(1LL << 21)}, .expected_length = 4},
                                         VariantTestCase{.value = int64_t{1LL << 28}, .expected_length = 5},
                                         VariantTestCase{.value = int64_t{-(1LL << 28)}, .expected_length = 5},
                                         VariantTestCase{.value = int64_t{1LL << 35}, .expected_length = 6},
                                         VariantTestCase{.value = int64_t{-(1LL << 35)}, .expected_length = 6},
                                         VariantTestCase{.value = int64_t{1LL << 42}, .expected_length = 7},
                                         VariantTestCase{.value = int64_t{-(1LL << 42)}, .expected_length = 7},
                                         VariantTestCase{.value = int64_t{1LL << 49}, .expected_length = 8},
                                         VariantTestCase{.value = int64_t{-(1LL << 49)}, .expected_length = 8},
                                         VariantTestCase{.value = int64_t{1LL << 56}, .expected_length = 9},
                                         VariantTestCase{.value = int64_t{-(1LL << 56)}, .expected_length = 9},
                                         VariantTestCase{.value = int64_t{(1LL << 62) - 1}, .expected_length = 9},
                                         VariantTestCase{.value = int64_t{std::numeric_limits<int64_t>::max()}, .expected_length = 10},
                                         VariantTestCase{.value = std::numeric_limits<int64_t>::min(), .expected_length = 10}));

static_assert(VarInt::kMaxVarIntLength<uint8_t> == VarInt::kMaxVarIntLength<int8_t>);
static_assert(VarInt::kMaxVarIntLength<uint16_t> == VarInt::kMaxVarIntLength<int16_t>);
static_assert(VarInt::kMaxVarIntLength<uint32_t> == VarInt::kMaxVarIntLength<int32_t>);
static_assert(VarInt::kMaxVarIntLength<uint64_t> == VarInt::kMaxVarIntLength<int64_t>);

static_assert(VarInt::kMaxVarIntLength<uint8_t> == 2);
static_assert(VarInt::kMaxVarIntLength<uint16_t> == 3);
static_assert(VarInt::kMaxVarIntLength<uint32_t> == 5);
static_assert(VarInt::kMaxVarIntLength<uint64_t> == 10);

}  // namespace