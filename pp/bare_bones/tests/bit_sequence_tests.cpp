#include <algorithm>
#include <random>

#include <gtest/gtest.h>

#include "bare_bones/bit_sequence.h"

namespace {

using BareBones::AllocationSize;
using BareBones::BitSequenceReader;
using BareBones::CompactBitSequence;

constexpr size_t NUM_VALUES = 1000;

constexpr std::array kAllocationSizesTable = {AllocationSize{0U}, AllocationSize{32U}, AllocationSize{64U}};

std::array<uint64_t, NUM_VALUES> generate_uint64_vector() {
  std::array<uint64_t, NUM_VALUES> data;
  std::ranges::generate(data, std::mt19937(testing::UnitTest::GetInstance()->random_seed()));
  return data;
}

std::array<double, NUM_VALUES> generate_double_vector() {
  std::array<double, NUM_VALUES> data;
  std::uniform_real_distribution<double> unif;
  std::mt19937 gen32(testing::UnitTest::GetInstance()->random_seed());
  std::ranges::generate(data, [&unif, gen32] mutable { return unif(gen32); });
  return data;
}

struct BitSequence : public testing::Test {
  BareBones::BitSequence bitseq;
};

TEST_F(BitSequence, SingleBit) {
  EXPECT_TRUE(bitseq.empty());
  EXPECT_EQ(bitseq.size(), 0);

  bitseq.push_back_single_zero_bit();
  EXPECT_EQ(bitseq.size(), 1);

  bitseq.push_back_single_one_bit();
  EXPECT_EQ(bitseq.size(), 2);

  EXPECT_FALSE(bitseq.empty());

  auto bitseq_reader = bitseq.reader();
  uint32_t outcome = bitseq_reader.read_bits_u32(1);
  EXPECT_EQ(outcome, 0);

  bitseq_reader.ff(1);
  outcome = bitseq_reader.read_bits_u32(1);
  EXPECT_EQ(outcome, 1);
  bitseq_reader.ff(1);

  EXPECT_EQ(bitseq_reader.left(), 0);

  bitseq.clear();
  EXPECT_TRUE(bitseq.empty());
  EXPECT_EQ(bitseq.size(), 0);
}

TEST_F(BitSequence, U64Svbyte0248) {
  EXPECT_TRUE(bitseq.empty());
  EXPECT_EQ(bitseq.size(), 0);

  const auto etalons = generate_uint64_vector();

  for (const auto& ts : etalons) {
    bitseq.push_back_u64_svbyte_0248(ts);
  }

  auto bitseq_reader = bitseq.reader();

  for (const auto& etalon : etalons) {
    auto outcome = bitseq_reader.consume_u64_svbyte_0248();
    EXPECT_EQ(outcome, etalon);
  }

  EXPECT_EQ(bitseq_reader.left(), 0);

  bitseq.clear();
  EXPECT_TRUE(bitseq.empty());
  EXPECT_EQ(bitseq.size(), 0);
}

TEST_F(BitSequence, U64Svbyte2468) {
  EXPECT_TRUE(bitseq.empty());
  EXPECT_EQ(bitseq.size(), 0);

  const auto etalons = generate_uint64_vector();

  for (const auto& ts : etalons) {
    bitseq.push_back_u64_svbyte_2468(ts);
  }

  auto bitseq_reader = bitseq.reader();

  for (const auto& etalon : etalons) {
    auto outcome = bitseq_reader.consume_u64_svbyte_2468();
    EXPECT_EQ(outcome, etalon);
  }

  EXPECT_EQ(bitseq_reader.left(), 0);

  bitseq.clear();
  EXPECT_TRUE(bitseq.empty());
  EXPECT_EQ(bitseq.size(), 0);
}

TEST_F(BitSequence, D64Svbyte2468) {
  EXPECT_TRUE(bitseq.empty());
  EXPECT_EQ(bitseq.size(), 0);

  const auto etalons = generate_double_vector();

  for (const auto& v : etalons) {
    bitseq.push_back_d64_svbyte_0468(v);
  }

  auto bitseq_reader = bitseq.reader();

  for (const auto& etalon : etalons) {
    auto outcome = bitseq_reader.consume_d64_svbyte_0468();
    EXPECT_EQ(outcome, etalon);
  }

  EXPECT_EQ(bitseq_reader.left(), 0);

  bitseq.clear();
  EXPECT_TRUE(bitseq.empty());
  EXPECT_EQ(bitseq.size(), 0);
}

TEST_F(BitSequence, TestPushBackU32) {
  // Arrange
  bitseq.push_back_single_one_bit();

  // Act
  bitseq.push_back_u32(0xFFFFFFFF);

  // Assert
  EXPECT_TRUE(std::ranges::equal(bitseq.bytes(), std::vector<uint8_t>{0xFF, 0xFF, 0xFF, 0xFF, 0x01}));
}

struct CopyWithSizeConstructorCase {
  uint8_t bits_count;
  uint32_t expected;
};

class BitSequenceCopyWithSizeConstructorFixture : public testing::TestWithParam<CopyWithSizeConstructorCase> {};

TEST_P(BitSequenceCopyWithSizeConstructorFixture, Test) {
  // Arrange
  BareBones::BitSequence stream;
  stream.push_back_u32(std::numeric_limits<uint32_t>::max());

  // Act
  const BareBones::BitSequence stream2(stream, GetParam().bits_count);

  // Assert
  ASSERT_EQ(GetParam().bits_count, stream2.size());
  EXPECT_EQ(GetParam().expected, *reinterpret_cast<const uint32_t*>(stream2.bytes().data()));
}

INSTANTIATE_TEST_SUITE_P(Tests,
                         BitSequenceCopyWithSizeConstructorFixture,
                         testing::Values(CopyWithSizeConstructorCase{.bits_count = 0, .expected = 0b0},
                                         CopyWithSizeConstructorCase{.bits_count = 1, .expected = 0b1},
                                         CopyWithSizeConstructorCase{.bits_count = 2, .expected = 0b11},
                                         CopyWithSizeConstructorCase{.bits_count = 3, .expected = 0b111},
                                         CopyWithSizeConstructorCase{.bits_count = 4, .expected = 0b1111},
                                         CopyWithSizeConstructorCase{.bits_count = 5, .expected = 0b11111},
                                         CopyWithSizeConstructorCase{.bits_count = 6, .expected = 0b111111},
                                         CopyWithSizeConstructorCase{.bits_count = 7, .expected = 0b1111111},
                                         CopyWithSizeConstructorCase{.bits_count = 8, .expected = 0b11111111},
                                         CopyWithSizeConstructorCase{.bits_count = 9, .expected = 0b111111111},
                                         CopyWithSizeConstructorCase{.bits_count = 16, .expected = 0b1111111111111111},
                                         CopyWithSizeConstructorCase{.bits_count = 32, .expected = std::numeric_limits<uint32_t>::max()}));

TEST_F(BitSequenceCopyWithSizeConstructorFixture, SourceStreamIsEmpty) {
  // Arrange
  const BareBones::BitSequence stream;

  // Act
  const BareBones::BitSequence stream2(stream, 0);

  // Assert
  EXPECT_TRUE(stream2.empty());
  EXPECT_TRUE(stream2.bytes().empty());
}

class CompactBitSequenceFixture : public testing::Test {
 protected:
  CompactBitSequence<kAllocationSizesTable> stream_;
};

TEST_F(CompactBitSequenceFixture, CopyConstructor) {
  // Arrange
  stream_.push_back_single_one_bit();

  // Act
  const auto stream2 = stream_;

  // Assert
  EXPECT_EQ(1U, stream_.size_in_bits());
  EXPECT_EQ(1U, stream2.size_in_bits());
  EXPECT_NE(stream_.raw_bytes(), stream2.raw_bytes());
  EXPECT_EQ(0b1U, stream_.bytes()[0]);
  EXPECT_EQ(0b1U, stream2.bytes()[0]);
}

TEST_F(CompactBitSequenceFixture, CopyOperator) {
  // Arrange
  stream_.push_back_single_one_bit();
  decltype(stream_) stream2;
  stream2.push_back_single_zero_bit();

  // Act
  stream2 = stream_;

  // Assert
  EXPECT_EQ(1U, stream_.size_in_bits());
  EXPECT_EQ(1U, stream2.size_in_bits());
  EXPECT_NE(stream_.raw_bytes(), stream2.raw_bytes());
  EXPECT_EQ(0b1U, stream2.bytes()[0]);
}

TEST_F(CompactBitSequenceFixture, CopyOperatorOnNonUniqueMemory) {
  // Arrange
  stream_.push_back_single_one_bit();
  decltype(stream_) stream2;
  stream2.push_back_single_zero_bit();
  const auto memory = stream2.shared_memory();

  // Act
  stream2 = stream_;

  // Assert
  EXPECT_EQ(1U, stream_.size_in_bits());
  EXPECT_EQ(1U, stream2.size_in_bits());
  EXPECT_NE(stream_.raw_bytes(), stream2.raw_bytes());
  EXPECT_NE(stream2.raw_bytes(), memory.get());
  EXPECT_EQ(0b1U, stream2.bytes()[0]);
  EXPECT_EQ(0b0U, memory.get()[0]);
}

TEST_F(CompactBitSequenceFixture, MoveConstructor) {
  // Arrange
  stream_.push_back_single_one_bit();

  // Act
  const auto stream2 = std::move(stream_);

  // Assert
  EXPECT_EQ(0U, stream_.size_in_bits());
  ASSERT_TRUE(stream_.bytes().empty());

  EXPECT_EQ(1U, stream2.size_in_bits());
  ASSERT_FALSE(stream2.bytes().empty());
  EXPECT_EQ(0b1U, stream2.bytes()[0]);
}

TEST_F(CompactBitSequenceFixture, MoveOperator) {
  // Arrange
  stream_.push_back_single_one_bit();

  // Act
  decltype(stream_) stream2;
  stream2.push_back_single_one_bit();
  stream2 = std::move(stream_);

  // Assert
  EXPECT_EQ(0U, stream_.size_in_bits());
  ASSERT_TRUE(stream_.bytes().empty());

  EXPECT_EQ(1U, stream2.size_in_bits());
  ASSERT_FALSE(stream2.bytes().empty());
  EXPECT_EQ(0b1U, stream2.bytes()[0]);
}

TEST_F(CompactBitSequenceFixture, MoveOperatorOnNonUniqueMemory) {
  // Arrange
  stream_.push_back_single_one_bit();
  const auto memory = stream_.shared_memory();
  decltype(stream_) stream2;
  stream2.push_back_single_zero_bit();
  const auto memory2 = stream2.shared_memory();

  // Act
  stream2 = std::move(stream_);

  // Assert
  EXPECT_EQ(0U, stream_.size_in_bits());
  ASSERT_TRUE(stream_.bytes().empty());

  EXPECT_EQ(1U, stream2.size_in_bits());
  ASSERT_FALSE(stream2.bytes().empty());
  EXPECT_EQ(0b1U, stream2.bytes()[0]);

  EXPECT_EQ(stream2.raw_bytes(), memory.get());
  EXPECT_EQ(0b0U, memory2.get()[0]);
}

TEST_F(CompactBitSequenceFixture, PushOnebit) {
  // Arrange

  // Act
  stream_.push_back_single_zero_bit();
  stream_.push_back_single_one_bit();
  stream_.push_back_single_zero_bit();
  stream_.push_back_single_one_bit();
  stream_.push_back_single_zero_bit();
  stream_.push_back_single_one_bit();
  stream_.push_back_single_zero_bit();
  stream_.push_back_single_one_bit();

  // Assert
  EXPECT_EQ(8U, stream_.size_in_bits());
  EXPECT_EQ(0b10101010, stream_.filled_bytes()[0]);
}

TEST_F(CompactBitSequenceFixture, PushUint32) {
  // Arrange

  // Act
  stream_.push_back_single_zero_bit();
  stream_.push_back_bits_u32(32, 0b10101010101010101010101010101010);
  const auto bytes = stream_.bytes<uint32_t>().data();

  // Assert
  ASSERT_EQ(33U, stream_.size_in_bits());
  EXPECT_EQ(0b01010101010101010101010101010100ULL, bytes[0]);
  EXPECT_EQ(0b1UL, bytes[1]);
}

TEST_F(CompactBitSequenceFixture, PushUint64) {
  // Arrange

  // Act
  stream_.push_back_single_zero_bit();
  stream_.push_back_u64(0b1010101010101010101010101010101010101010101010101010101010101010);
  const auto bytes = stream_.bytes<uint64_t>().data();

  // Assert
  ASSERT_EQ(65U, stream_.size_in_bits());
  EXPECT_EQ(0b0101010101010101010101010101010101010101010101010101010101010100ULL, bytes[0]);
  EXPECT_EQ(0b1ULL, bytes[1]);
}

TEST_F(CompactBitSequenceFixture, PushUint64_2) {
  // Arrange

  // Act
  stream_.push_back_u64(0b1010101010101010101010101010101010101010101010101010101010101010);
  const auto bytes = stream_.bytes<uint64_t>().data();

  // Assert
  ASSERT_EQ(64U, stream_.size_in_bits());
  EXPECT_EQ(0b1010101010101010101010101010101010101010101010101010101010101010, bytes[0]);
}

TEST_F(CompactBitSequenceFixture, PushBackBytesAligned) {
  // Arrange
  std::array<uint8_t, 15> push_bytes = {0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14};

  // Act
  stream_.push_back_bytes(push_bytes.data(), BareBones::Bit::to_bits(push_bytes.size()));
  auto bytes = stream_.bytes<uint8_t>();

  // Assert
  EXPECT_EQ(push_bytes.size(), bytes.size());
  EXPECT_TRUE(std::ranges::equal(push_bytes, bytes));
}

TEST_F(CompactBitSequenceFixture, PushBackBytesSpan) {
  // Arrange
  std::array<uint8_t, 15> push_bytes = {0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14};

  // Act
  stream_.push_back_bytes(push_bytes);
  auto bytes = stream_.bytes<uint8_t>();

  // Assert
  EXPECT_EQ(push_bytes.size(), bytes.size());
  EXPECT_TRUE(std::ranges::equal(push_bytes, bytes));
}

TEST_F(CompactBitSequenceFixture, PushBackBytesUnaligned) {
  // Arrange
  const std::array<uint8_t, 15> push_bytes_init = {0b01010101, 0b01010101, 0b01010101, 0b01010101, 0b01010101, 0b01010101, 0b01010101, 0b01010101,
                                                   0b01010101, 0b01010101, 0b01010101, 0b01010101, 0b01010101, 0b01010101, 0b01010101};
  const std::array<uint8_t, 16> push_bytes_rest = {0b10101010, 0b10101010, 0b10101010, 0b10101010, 0b10101010, 0b10101010, 0b10101010, 0b10101010,
                                                   0b10101010, 0b10101010, 0b10101010, 0b10101010, 0b10101010, 0b10101010, 0b10101010, 0b0};

  // Act
  stream_.push_back_single_zero_bit();
  stream_.push_back_bytes(push_bytes_init.data(), BareBones::Bit::to_bits(push_bytes_init.size()));
  auto bytes = stream_.bytes<uint8_t>();

  // Assert
  EXPECT_EQ(push_bytes_rest.size(), bytes.size());
  EXPECT_TRUE(std::ranges::equal(push_bytes_rest, bytes));
}

TEST_F(CompactBitSequenceFixture, TrimUint32) {
  // Arrange

  // Act
  stream_.push_back_bits_u32(32, 0b10101010101010101010101010101010);
  stream_.trim_lower_bytes(2);
  const auto bytes = stream_.bytes<uint16_t>().data();

  // Assert
  ASSERT_EQ(16U, stream_.size_in_bits());
  EXPECT_EQ(0b1010101010101010ULL, bytes[0]);
}

TEST_F(CompactBitSequenceFixture, TrimUint32_2) {
  // Arrange

  // Act
  stream_.push_back_single_zero_bit();
  stream_.push_back_bits_u32(32, 0b10101010101010101010101010101010);

  stream_.trim_lower_bytes(4);
  const auto bytes = stream_.bytes<uint16_t>().data();

  // Assert
  ASSERT_EQ(1U, stream_.size_in_bits());
  EXPECT_EQ(0b1ULL, bytes[0]);
}

TEST_F(CompactBitSequenceFixture, TrimUint32_3) {
  // Arrange

  // Act
  stream_.push_back_single_zero_bit();
  stream_.push_back_bits_u32(32, 0b10101010101010101010101010101010);

  stream_.trim_lower_bytes(3);
  const auto bytes = stream_.bytes<uint16_t>().data();

  // Assert
  ASSERT_EQ(9U, stream_.size_in_bits());
  EXPECT_EQ(0b101010101ULL, bytes[0]);
}

TEST_F(CompactBitSequenceFixture, TrimUint64) {
  // Arrange

  // Act
  stream_.push_back_u64(0b1010101010101010101010101010101010101010101010101010101010101010);
  stream_.trim_lower_bytes(5);
  const auto bytes = stream_.bytes<uint32_t>().data();

  // Assert
  ASSERT_EQ(24U, stream_.size_in_bits());
  EXPECT_EQ(0b101010101010101010101010ULL, bytes[0]);
}

TEST_F(CompactBitSequenceFixture, TrimUint64_2) {
  // Arrange

  // Act
  stream_.push_back_single_zero_bit();
  stream_.push_back_u64(0b1010101010101010101010101010101010101010101010101010101010101010);
  stream_.trim_lower_bytes(8);
  const auto bytes = stream_.bytes<uint64_t>().data();

  // Assert
  ASSERT_EQ(1U, stream_.size_in_bits());
  EXPECT_EQ(0b1ULL, bytes[0]);
}

TEST_F(CompactBitSequenceFixture, ShrinkToFit) {
  // Arrange
  static constexpr auto kValue = std::numeric_limits<uint64_t>::max();

  stream_.push_back_u64(kValue);
  const auto allocated_memory = stream_.allocated_memory();

  // Act
  stream_.shrink_to_fit();

  // Assert
  EXPECT_LT(stream_.allocated_memory(), allocated_memory);
  ASSERT_EQ(sizeof(kValue), stream_.size_in_bytes());
  EXPECT_EQ(kValue, *stream_.bytes<uint64_t>().data());
}

TEST_F(CompactBitSequenceFixture, ShrinkToFitOnNonUniqueMemory) {
  // Arrange
  static constexpr auto kValue = std::numeric_limits<uint64_t>::max();

  stream_.push_back_u64(kValue);
  const auto memory = stream_.shared_memory();
  const auto memory_size = stream_.size_in_bits();
  const auto allocated_memory = stream_.allocated_memory();

  // Act
  stream_.shrink_to_fit();

  // Assert
  EXPECT_LT(stream_.allocated_memory(), allocated_memory);
  ASSERT_EQ(sizeof(kValue), stream_.size_in_bytes());
  EXPECT_EQ(kValue, *stream_.bytes<uint64_t>().data());
  ASSERT_EQ(BareBones::Bit::to_bits(sizeof(kValue)), memory_size);
  EXPECT_EQ(kValue, *reinterpret_cast<uint64_t*>(memory.get()));
}

TEST_F(CompactBitSequenceFixture, ReallocOnNonUniqueMemory) {
  // Arrange
  static constexpr auto kValue = std::numeric_limits<uint64_t>::max();

  stream_.push_back_u64(kValue);
  stream_.push_back_u64(kValue);
  stream_.push_back_u64(kValue);
  const auto memory = stream_.shared_memory();
  const auto memory_size = stream_.size_in_bits();

  // Act
  stream_.push_back_u64(kValue);

  // Assert
  ASSERT_EQ(BareBones::Bit::to_bits(sizeof(kValue) * 3), memory_size);

  // NOLINTNEXTLINE(clang-analyzer-unix.Malloc)
  EXPECT_NE(stream_.raw_bytes(), memory.get());
  // NOLINTNEXTLINE(clang-analyzer-unix.Malloc)
  EXPECT_TRUE(std::ranges::equal(std::vector{kValue, kValue, kValue}, std::span(reinterpret_cast<uint64_t*>(memory.get()), 3)));
  EXPECT_TRUE(std::ranges::equal(std::vector{kValue, kValue, kValue, kValue}, stream_.bytes<uint64_t>()));
}

template <class T>
class BitSequenceReaderFixture : public testing::Test {};

using BitSequenceTypes = testing::Types<BareBones::BitSequence, CompactBitSequence<kAllocationSizesTable>>;
TYPED_TEST_SUITE(BitSequenceReaderFixture, BitSequenceTypes);

TYPED_TEST(BitSequenceReaderFixture, read_bits_u32) {
  // Arrange
  constexpr uint32_t value = 0xAABBCCDD;
  TypeParam stream;
  stream.push_back_bits_u32(32, value);

  // Act
  auto reader = stream.reader();
  auto dd = reader.consume_bits_u32(8);
  auto cc = reader.consume_bits_u32(8);
  auto bb = reader.consume_bits_u32(8);
  auto aa = reader.consume_bits_u32(8);

  // Assert
  EXPECT_EQ(aa, 0xAA);
  EXPECT_EQ(bb, 0xBB);
  EXPECT_EQ(cc, 0xCC);
  EXPECT_EQ(dd, 0xDD);
}

TYPED_TEST(BitSequenceReaderFixture, read_u64) {
  // Arrange
  constexpr auto value = 0b0101010101010101010101010101010101010101010101010101010101010101U;
  TypeParam stream;
  stream.push_back_u64(value);

  // Act
  auto reader = stream.reader();

  // Assert
  EXPECT_EQ(value, reader.read_u64());
}

TYPED_TEST(BitSequenceReaderFixture, read_u64_2) {
  // Arrange
  constexpr auto value = 0b0101010101010101010101010101010101010101010101010101010101010101U;
  TypeParam stream;
  stream.push_back_single_zero_bit();
  stream.push_back_u64(value);

  // Act
  auto reader = stream.reader();
  reader.ff(1);

  // Assert
  EXPECT_EQ(value, reader.read_u64());
}

TYPED_TEST(BitSequenceReaderFixture, read_bits_u64) {
  // Arrange
  constexpr auto value = 0b0101010101010101010101010101010101010101010101010101010101010101U;
  TypeParam stream;
  stream.push_back_u64(value);

  // Act
  auto reader = stream.reader();

  // Assert
  EXPECT_EQ(value, reader.read_bits_u64(BareBones::Bit::to_bits(sizeof(uint64_t))));
}

TYPED_TEST(BitSequenceReaderFixture, read_bits_u64_2) {
  // Arrange
  constexpr auto value = 0b0101010101010101010101010101010101010101010101010101010101010101U;
  TypeParam stream;
  stream.push_back_single_zero_bit();
  stream.push_back_u64(value);

  // Act
  auto reader = stream.reader();
  reader.ff(1);

  // Assert
  EXPECT_EQ(value, reader.read_bits_u64(BareBones::Bit::to_bits(sizeof(uint64_t))));
}

}  // namespace
