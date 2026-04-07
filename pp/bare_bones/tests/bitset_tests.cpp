#include <algorithm>
#include <random>
#include <spanstream>
#include <string>
#include <vector>

#include <gtest/gtest.h>

#include "bare_bones/bitset.h"

namespace {

using std::operator""sv;

class BitsetFixture : public testing::Test {
 protected:
  BareBones::Bitset bs_;
};

TEST_F(BitsetFixture, valid_unset) {
  constexpr int size = 65536;

  bs_.resize(size);
  for (auto i = 0; i < size; ++i) {
    bs_.set(i);
  }

  bs_.clear();
  ASSERT_EQ(std::ranges::distance(bs_.begin(), bs_.end()), 0);
}

TEST_F(BitsetFixture, random_access) {
  // Arrange
  constexpr std::array indexes = {0U, 134U, 1087U, 5378U, 12098U, 65535U};
  static_assert(std::ranges::is_sorted(indexes));

  // Act
  bs_.resize(indexes.back() + 1);
  for (const auto index : indexes) {
    bs_.set(index);
  }

  // Assert
  EXPECT_TRUE(std::ranges::equal(bs_, indexes));
}

TEST_F(BitsetFixture, should_decriase_increase_size) {
  bs_.resize(1024);
  // set every odd bit
  for (auto i = 0; i < 1024; ++i) {
    if (i % 2) {
      bs_.set(i);
    }
  }

  // Check decrease resize
  // after resize we have bitset {0,1}
  bs_.resize(2);
  ASSERT_EQ(bs_.size(), 2U);

  // Check increase resize
  bs_.resize(4);
  // after resize we have bitset {0,1,0,0}
  // create vector that include all position of set bits
  const std::vector<uint32_t> bs_vec_reference = {1};

  // save possition of set original bitset to vector
  std::vector<uint32_t> bs_vec;
  for (auto i : bs_) {
    bs_vec.push_back(i);
  }

  ASSERT_EQ(bs_vec, bs_vec_reference);
  ASSERT_EQ(bs_.size(), 4U);
}

TEST_F(BitsetFixture, should_iterate_over_empty) {
  uint32_t count = 0;
  for (const auto i : bs_) {
    count += i;
  }

  ASSERT_EQ(count, 0U);
}

TEST_F(BitsetFixture, should_iterate_over_resized_empty) {
  bs_.resize(10);
  bs_.resize(0);

  uint32_t count = 0;
  for (const auto i : bs_) {
    count += i;
  }

  ASSERT_EQ(count, 0U);
}

TEST_F(BitsetFixture, should_not_out_of_range_on_resize) {
  bs_.set(2047);
  bs_.resize(4096);

  uint32_t count = 0;
  for (const auto i : bs_) {
    count += i;
  }

  ASSERT_EQ(count, 2047U);
}

TEST_F(BitsetFixture, TestEmptyIsEmpty) {
  // Arrange

  // Act

  // Assert
  EXPECT_TRUE(bs_.empty());
}

TEST_F(BitsetFixture, TestNotFilledIsEmpty) {
  // Arrange
  bs_.resize(10);
  // Act

  // Assert
  EXPECT_TRUE(bs_.empty());
}

TEST_F(BitsetFixture, TestNotEmpty) {
  // Arrange
  bs_.set(10);

  // Act

  // Assert
  EXPECT_FALSE(bs_.empty());
}

TEST_F(BitsetFixture, ResetInFirstUint64) {
  // Arrange

  // Act
  bs_.set(0);
  bs_.reset(0);

  // Assert
  EXPECT_FALSE(bs_[0]);
}

TEST_F(BitsetFixture, ResetInSecondUint64) {
  // Arrange

  // Act
  bs_.set(64);
  bs_.reset(64);

  // Assert
  EXPECT_FALSE(bs_[64]);
}

TEST_F(BitsetFixture, TestResetCorrectness) {
  // Arrange

  // Act
  bs_.set(0);
  bs_.set(1);
  bs_.set(2);
  bs_.reset(1);

  // Assert
  EXPECT_TRUE(bs_[0]);
  EXPECT_FALSE(bs_[1]);
  EXPECT_TRUE(bs_[2]);
}

TEST_F(BitsetFixture, TestSetAtomicCorrectness) {
  // Arrange
  bs_.resize(8);

  // Act
  bs_.set_atomic(7);

  // Assert
  EXPECT_TRUE(bs_[7]);
}

TEST_F(BitsetFixture, TestResetAtomicCorrectness) {
  // Arrange
  bs_.resize(8);
  bs_.set_atomic(7);

  // Act
  bs_.reset_atomic(7);

  // Assert
  EXPECT_FALSE(bs_[7]);
}

TEST_F(BitsetFixture, TestSetIter) {
  // Arrange
  const std::array<uint32_t, 2> idx = {0, 2};

  // Act
  bs_.set(idx.begin(), idx.end());

  // Assert
  EXPECT_TRUE(bs_[0]);
  EXPECT_FALSE(bs_[1]);
  EXPECT_TRUE(bs_[2]);
}

TEST_F(BitsetFixture, TestSetInitList) {
  // Arrange
  // Act
  bs_.set({0, 2});

  // Assert
  EXPECT_TRUE(bs_[0]);
  EXPECT_FALSE(bs_[1]);
  EXPECT_TRUE(bs_[2]);
}

TEST_F(BitsetFixture, TestResetIter) {
  // Arrange
  bs_.set({0, 1, 2});
  const std::array<uint32_t, 2> idx = {0, 2};

  // Act
  bs_.reset(idx.begin(), idx.end());

  // Assert
  EXPECT_FALSE(bs_[0]);
  EXPECT_TRUE(bs_[1]);
  EXPECT_FALSE(bs_[2]);
}

TEST_F(BitsetFixture, TestResetInitList) {
  // Arrange
  bs_.set({0, 1, 2});

  // Act
  bs_.reset({0, 2});

  // Assert
  EXPECT_FALSE(bs_[0]);
  EXPECT_TRUE(bs_[1]);
  EXPECT_FALSE(bs_[2]);
}

TEST_F(BitsetFixture, PopcountOnEmptyBitset) {
  // Arrange

  // Act

  // Assert
  EXPECT_EQ(0U, bs_.popcount());
}

TEST_F(BitsetFixture, PopcountOnNonEmptyBitset) {
  // Arrange

  // Act
  bs_.set(0);
  bs_.set(4);
  bs_.set(8);

  // Assert
  EXPECT_EQ(3U, bs_.popcount());
}

TEST_F(BitsetFixture, PopcountAfterResizeInCurrentUint64) {
  // Arrange

  // Act
  bs_.set(0);
  bs_.resize(0);

  // Assert
  EXPECT_EQ(0U, bs_.popcount());
}

TEST_F(BitsetFixture, PopcountAfterResizeInNextUint64) {
  // Arrange

  // Act
  bs_.set(8);
  bs_.resize(0);
  bs_.resize(9);

  // Assert
  EXPECT_EQ(0U, bs_.popcount());
}

TEST_F(BitsetFixture, IterateOverEmptyBitset) {
  // Arrange

  // Act

  // Assert
  EXPECT_EQ(0U, std::ranges::distance(bs_));
}

TEST_F(BitsetFixture, ZeroIteratorReturnsAllUnsetIdsInBitset) {
  // Arrange
  bs_.resize(8);
  bs_.set({1, 3, 6});
  const std::vector<uint32_t> expected{0, 2, 4, 5, 7};

  // Act
  std::vector<uint32_t> actual;
  for (auto it = bs_.zbegin(); it != bs_.zend(); ++it) {
    actual.push_back(*it);
  }

  // Assert
  EXPECT_EQ(expected, actual);
}

TEST_F(BitsetFixture, ZeroIteratorSupportsExternalBoundaryFiltering) {
  // Arrange
  bs_.resize(10);
  bs_.set({0, 3, 4, 7, 9});
  const std::vector<uint32_t> expected{5, 6, 8};

  // Act
  std::vector<uint32_t> actual;
  for (auto it = bs_.zbegin(); it != bs_.zend(); ++it) {
    if (*it >= 9) {
      break;
    }
    if (*it >= 5) {
      actual.push_back(*it);
    }
  }

  // Assert
  EXPECT_EQ(expected, actual);
}

TEST_F(BitsetFixture, ZeroCountMatchesUnsetBitsInBitset) {
  // Arrange
  bs_.resize(10);
  bs_.set({0, 1, 4, 8});

  // Act
  const auto zero_count_full = bs_.zerocount();

  // Assert
  EXPECT_EQ(6U, zero_count_full);
}

class BitsetCreateIteratorFixture : public testing::Test {
 protected:
  std::vector<uint8_t> bytes_data_;
};

TEST_F(BitsetCreateIteratorFixture, CreateReadIteratorLess4Bytes) {
  // Arrange
  bytes_data_ = {0x00, 0x00, 0x00};
  std::span<const uint8_t> buffer(bytes_data_);

  // Act
  const auto it = BareBones::Bitset::create_read_iterator(buffer);

  // Assert
  EXPECT_EQ(it, BareBones::Bitset::IteratorSentinel{});
  EXPECT_EQ(buffer.size(), 3);
}

TEST_F(BitsetCreateIteratorFixture, CreateReadIteratorWrongSize) {
  // Arrange
  bytes_data_ = {0x00, 0x00, 0x00, 0x01, 0x00};
  std::span<const uint8_t> buffer(bytes_data_);

  // Act
  const auto it = BareBones::Bitset::create_read_iterator(buffer);

  // Assert
  EXPECT_EQ(it, BareBones::Bitset::IteratorSentinel{});
  EXPECT_EQ(buffer.size(), 1);
}

class BitsetCreateIteratorValidFixture : public testing::Test {
 protected:
  BareBones::Bitset bs_;
  BareBones::ShrinkedToFitOStringStream stream_;
};

TEST_F(BitsetCreateIteratorValidFixture, TestWriteSize) {
  // Arrange
  bs_.set({1, 10, 100, 1000});

  // Act
  bs_.write_to(stream_);

  // Assert
  EXPECT_EQ(stream_.view().size(), bs_.get_write_size());
}

TEST_F(BitsetCreateIteratorValidFixture, CreateReadIteratorValid) {
  // Arrange
  bs_.set({1, 10, 100, 1000});
  bs_.write_to(stream_);
  std::span buffer = stream_.span<const uint8_t>();

  // Act
  const auto it = BareBones::Bitset::create_read_iterator(buffer);

  // Assert
  EXPECT_TRUE(std::ranges::equal(it, BareBones::Bitset::IteratorSentinel{}, bs_.begin(), bs_.end()));
  EXPECT_EQ(buffer.size(), 0);
}

class BitsetReadFromFixture : public testing::Test {
 protected:
  BareBones::Bitset bs_;
};

TEST_F(BitsetReadFromFixture, ReadSizeError) {
  // Arrange
  std::ispanstream stream{""};

  // Act
  const auto result = bs_.read_from(stream);

  // Assert
  EXPECT_FALSE(result);
}

TEST_F(BitsetReadFromFixture, ReadBytesError) {
  // Arrange
  std::ispanstream stream{"\x01\x00\x00\x00"sv};

  // Act
  const auto result = bs_.read_from(stream);

  // Assert
  EXPECT_FALSE(result);
}

TEST_F(BitsetReadFromFixture, ReadEmptyBitset) {
  // Arrange
  std::ispanstream stream{"\x00\x00\x00\x00"sv};

  // Act
  const auto result = bs_.read_from(stream);

  // Assert
  EXPECT_TRUE(result);
}

TEST_F(BitsetReadFromFixture, ReadSuccess) {
  // Arrange
  std::ispanstream stream{"\x01\x00\x00\x00\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF"sv};

  // Act
  const auto result = bs_.read_from(stream);

  // Assert
  EXPECT_TRUE(result);
  EXPECT_EQ(1U, bs_.size());
  EXPECT_TRUE(bs_.is_set(0));
}

TEST_F(BitsetReadFromFixture, OverwriteBitsetAfterRead) {
  // Arrange
  bs_.set({1, 2, 3, 4, 5});
  std::ispanstream stream{"\x01\x00\x00\x00\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF"sv};

  // Act
  const auto result = bs_.read_from(stream);

  // Assert
  EXPECT_TRUE(result);
  EXPECT_EQ(1U, bs_.size());
  EXPECT_TRUE(bs_.is_set(0));
}

class BitsetConstructorsFixture : public BitsetFixture {};

TEST_F(BitsetConstructorsFixture, CopyConstructor) {
  // Arrange
  bs_.resize(1001);
  bs_.set(1);
  bs_.set(100);
  bs_.set(1000);

  // Act
  BareBones::Bitset bs_copy(bs_);

  // Assert
  EXPECT_TRUE(std::ranges::equal(bs_, bs_copy));
}

TEST_F(BitsetConstructorsFixture, MoveConstructor) {
  // Arrange
  bs_.resize(1001);
  bs_.set(1);
  bs_.set(100);
  bs_.set(1000);

  // Act
  BareBones::Bitset bs_move(std::move(bs_));

  // Assert
  EXPECT_TRUE(std::ranges::equal(bs_move, std::initializer_list<uint32_t>{1, 100, 1000}));
}

TEST_F(BitsetConstructorsFixture, CopyAssignment) {
  // Arrange
  bs_.resize(1001);

  bs_.set(1);
  bs_.set(100);
  bs_.set(1000);

  // Act
  BareBones::Bitset bs_copy = bs_;

  // Assert
  EXPECT_TRUE(std::ranges::equal(bs_, bs_copy));
}

TEST_F(BitsetConstructorsFixture, CopyAssignmentNonEmpty) {
  // Arrange
  bs_.resize(1001);

  bs_.set(1);
  bs_.set(100);
  bs_.set(1000);

  BareBones::Bitset bs_copy;
  bs_copy.resize(3);
  bs_copy.set(0);
  bs_copy.set(1);
  bs_copy.set(2);

  // Act
  bs_copy = bs_;

  // Assert
  EXPECT_TRUE(std::ranges::equal(bs_, bs_copy));
}

TEST_F(BitsetConstructorsFixture, MoveAssignment) {
  // Arrange
  bs_.resize(1001);

  bs_.set(1);
  bs_.set(100);
  bs_.set(1000);

  // Act
  BareBones::Bitset bs_move = std::move(bs_);

  // Assert
  EXPECT_TRUE(std::ranges::equal(bs_move, std::initializer_list<uint32_t>{1, 100, 1000}));
}

TEST_F(BitsetConstructorsFixture, MoveAssignmentNonEmpty) {
  // Arrange
  bs_.resize(1001);

  bs_.set(1);
  bs_.set(100);
  bs_.set(1000);

  BareBones::Bitset bs_move;
  bs_move.resize(3);
  bs_move.set(0);
  bs_move.set(1);
  bs_move.set(2);

  // Act
  bs_move = std::move(bs_);

  // Assert
  EXPECT_TRUE(std::ranges::equal(bs_move, std::initializer_list<uint32_t>{1, 100, 1000}));
}

}  // namespace
