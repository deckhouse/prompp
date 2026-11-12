#include <array>
#include <cstring>
#include <iterator>
#include <limits>
#include <memory>
#include <span>
#include <sstream>
#include <vector>

#include <ranges>

#include <gmock/gmock.h>

#include "bare_bones/stream_v_byte.h"

namespace {

using BareBones::SharedPtrControlBlockWithItemCount;
using BareBones::StreamVByte::CompactSequence;
using BareBones::StreamVByte::Sequence;

constexpr std::array<uint64_t, 12> k64BitBoundaryValues{1,        255,        256,        65535,         65536,         16777215,
                                                        16777216, 4294967295, 4294967296, 1099511627775, 1099511627776, 9223372036854775807};

template <class T>
class StreamVByte64BitSequenceFixture : public testing::Test {};

using SequenceCodecTypes = testing::Types<Sequence<BareBones::StreamVByte::Codec1238>, Sequence<BareBones::StreamVByte::Codec1238Mostly1>>;
TYPED_TEST_SUITE(StreamVByte64BitSequenceFixture, SequenceCodecTypes);

TYPED_TEST(StreamVByte64BitSequenceFixture, PreservesValuesAtEncodingWidthBoundaries) {
  // Arrange
  TypeParam outcomes;

  // Act
  std::ranges::copy(k64BitBoundaryValues, std::back_inserter(outcomes));

  // Assert
  EXPECT_TRUE(std::ranges::equal(k64BitBoundaryValues, outcomes));
}

template <class T>
class StreamVByte64BitContainerFixture : public testing::Test {};

using ContainerCodecTypes = testing::Types<std::vector<uint8_t>, BareBones::Vector<uint8_t>>;

TYPED_TEST_SUITE(StreamVByte64BitContainerFixture, ContainerCodecTypes);

TYPED_TEST(StreamVByte64BitContainerFixture, PreservesValuesAtEncodingWidthBoundaries) {
  // Arrange
  TypeParam outcomes;
  auto out = BareBones::StreamVByte::back_inserter<BareBones::StreamVByte::Codec1238>(outcomes, k64BitBoundaryValues.size());

  // Act
  std::ranges::copy(k64BitBoundaryValues, out);

  auto [outcomes_b_i, outcomes_e_i] = BareBones::StreamVByte::decoder<BareBones::StreamVByte::Codec1238>(outcomes.begin(), k64BitBoundaryValues.size());

  // Assert
  EXPECT_TRUE(std::ranges::equal(k64BitBoundaryValues.begin(), k64BitBoundaryValues.end(), outcomes_b_i, outcomes_e_i));
}

struct IotaCase {
  const char* name;
  std::ranges::iota_view<uint32_t, uint32_t> values;
};

class SequenceIotaFixture : public ::testing::TestWithParam<IotaCase> {
 protected:
  Sequence<BareBones::StreamVByte::Codec0124, 4> sequence_;
};

TEST_P(SequenceIotaFixture, PreservesValuesAcrossBlockBoundaries) {
  // Arrange

  // Act
  std::ranges::copy(GetParam().values, std::back_insert_iterator(sequence_));

  // Assert
  EXPECT_TRUE(std::ranges::equal(GetParam().values, sequence_));
}

class CompactSequenceIotaFixture : public ::testing::TestWithParam<IotaCase> {
 protected:
  CompactSequence<BareBones::StreamVByte::Codec0124, BareBones::MemoryWithItemCount, 4> sequence_;
};

TEST_P(CompactSequenceIotaFixture, PreservesValuesAcrossBlockBoundaries) {
  // Arrange

  // Act
  std::ranges::copy(GetParam().values, std::back_insert_iterator(sequence_));

  // Assert
  EXPECT_TRUE(std::ranges::equal(GetParam().values, sequence_));
}

const auto kIotaCases = testing::Values(
    IotaCase{.name = "Empty", .values = std::views::iota(0U, 0U)},
    IotaCase{.name = "BeforeFirstBlock", .values = std::views::iota(0U, 3U)},
    IotaCase{.name = "AtFirstBlock", .values = std::views::iota(0U, 4U)},
    IotaCase{.name = "AfterFirstBlock", .values = std::views::iota(0U, 5U)},
    IotaCase{.name = "AtSecondBlock", .values = std::views::iota(0U, 8U)},
    IotaCase{.name = "AfterSecondBlock", .values = std::views::iota(0U, 9U)},
    IotaCase{.name = "MaximumValues", .values = std::views::iota(std::numeric_limits<uint32_t>::max() - 4, std::numeric_limits<uint32_t>::max())});

const auto kIotaCaseName = [](const testing::TestParamInfo<IotaCase>& info) { return info.param.name; };
INSTANTIATE_TEST_SUITE_P(Cases, SequenceIotaFixture, kIotaCases, kIotaCaseName);
INSTANTIATE_TEST_SUITE_P(Cases, CompactSequenceIotaFixture, kIotaCases, kIotaCaseName);

class ReadonlyCompactSequenceFixture : public testing::Test {
 protected:
  template <class T>
  using SharedMemory = BareBones::SharedMemory<T, SharedPtrControlBlockWithItemCount, BareBones::DefaultReallocator>;

  template <class T>
  using SharedSpan = BareBones::SharedSpan<T, SharedPtrControlBlockWithItemCount, BareBones::DefaultReallocator>;

  using Codec = BareBones::StreamVByte::Codec0124;

  using CompactSequenceSnapshot = CompactSequence<Codec, SharedSpan, 4>;

  CompactSequence<Codec, SharedMemory, 4> sequence_;
};

TEST_F(ReadonlyCompactSequenceFixture, SnapshotPreservesValues) {
  // Arrange
  static constexpr std::array kValues{0U, 1U, 2U, 3U};

  std::ranges::copy(kValues, std::back_inserter(sequence_));

  // Act
  CompactSequenceSnapshot snapshot(sequence_);

  // Assert
  EXPECT_TRUE(std::ranges::equal(kValues, snapshot));
}

TEST_F(ReadonlyCompactSequenceFixture, MoveConstructionTransfersSnapshotValues) {
  // Arrange
  sequence_.push_back(7);
  auto snapshot = sequence_.snapshot();

  // Act
  const auto moved = std::move(snapshot);

  // Assert
  EXPECT_THAT(moved.view(), testing::ElementsAre(7));
  // The move contract keeps the source empty and readable.
  // NOLINTNEXTLINE(bugprone-use-after-move, clang-analyzer-cplusplus.Move)
  EXPECT_THAT(snapshot.view(), testing::ElementsAre());
}

TEST_F(ReadonlyCompactSequenceFixture, MoveAssignmentReplacesSnapshotValues) {
  // Arrange
  sequence_.push_back(7);
  auto snapshot = sequence_.snapshot();
  sequence_.push_back(9);
  auto moved = sequence_.snapshot();

  // Act
  moved = std::move(snapshot);

  // Assert
  EXPECT_THAT(moved.view(), testing::ElementsAre(7));
  // The move contract keeps the source empty and readable.
  // NOLINTNEXTLINE(bugprone-use-after-move, clang-analyzer-cplusplus.Move)
  EXPECT_THAT(snapshot.view(), testing::ElementsAre());
}

TEST_F(ReadonlyCompactSequenceFixture, SnapshotIteratorsStayEqualAfterAppend) {
  // Arrange
  sequence_.push_back(0);
  const auto snapshot = sequence_.snapshot();
  const auto before = snapshot.view().begin();

  // Act
  sequence_.push_back(256);
  const auto after = snapshot.view().begin();

  // Assert
  EXPECT_EQ(before, after);
  EXPECT_THAT(snapshot.view(), testing::ElementsAre(0));
}

TEST_F(ReadonlyCompactSequenceFixture, SnapshotEndIteratorsStayEqualAfterAppend) {
  // Arrange
  const std::array values{0U, 0U, 0U};
  std::ranges::copy(values, std::back_inserter(sequence_));
  const auto snapshot = sequence_.snapshot();
  auto before = snapshot.view().begin();

  // Act
  sequence_.push_back(256);
  auto after = snapshot.view().begin();
  std::ranges::advance(before, 3);
  std::ranges::advance(after, 3);

  // Assert
  EXPECT_EQ(before, after);
  EXPECT_EQ(BareBones::StreamVByte::DecodeIteratorSentinel{}, before);
  EXPECT_EQ(BareBones::StreamVByte::DecodeIteratorSentinel{}, after);
}

class CompactSequenceCreateIteratorFixture : public testing::Test {
 protected:
  CompactSequence<BareBones::StreamVByte::Codec0124, BareBones::MemoryWithItemCount, 4> sequence_;
};

TEST_F(CompactSequenceCreateIteratorFixture, RejectsTruncatedHeaderWithoutConsumingInput) {
  // Arrange
  std::array<uint8_t, 6> data{};
  std::span<const uint8_t> buffer(data);

  // Act
  const auto it = sequence_.create_read_iterator(buffer);

  // Assert
  EXPECT_EQ(BareBones::StreamVByte::DecodeIteratorSentinel{}, it);
  EXPECT_EQ(data.data(), buffer.data());
  EXPECT_EQ(data.size(), buffer.size());
}

TEST_F(CompactSequenceCreateIteratorFixture, RejectsTruncatedBodyAfterConsumingHeader) {
  // Arrange
  const std::array<uint8_t, 9> record{3, 0, 0, 0, 1, 0, 0, 0, 0};
  std::span<const uint8_t> buffer(record);

  // Act
  const auto it = sequence_.create_read_iterator(buffer);

  // Assert
  EXPECT_EQ(BareBones::StreamVByte::DecodeIteratorSentinel{}, it);
  EXPECT_EQ(record.data() + 8, buffer.data());
  EXPECT_EQ(1U, buffer.size());
}

TEST_F(CompactSequenceCreateIteratorFixture, RejectsNonzeroCountWithoutBody) {
  // Arrange
  const std::array<uint8_t, 8> record{0, 0, 0, 0, 1, 0, 0, 0};
  std::span<const uint8_t> bytes(record);

  // Act
  const auto it = sequence_.create_read_iterator(bytes);

  // Assert
  EXPECT_EQ(BareBones::StreamVByte::DecodeIteratorSentinel{}, it);
  EXPECT_TRUE(bytes.empty());
}

TEST_F(CompactSequenceCreateIteratorFixture, RejectsMissingValueBytes) {
  // Arrange
  const std::array<uint8_t, 9> record{1, 0, 0, 0, 1, 0, 0, 0, 3};
  std::span<const uint8_t> bytes(record);

  // Act
  const auto it = sequence_.create_read_iterator(bytes);

  // Assert
  EXPECT_EQ(BareBones::StreamVByte::DecodeIteratorSentinel{}, it);
  EXPECT_TRUE(bytes.empty());
}

TEST_F(CompactSequenceCreateIteratorFixture, RejectsCountBeyondAvailableBlocks) {
  // Arrange
  const std::array<uint8_t, 9> record{1, 0, 0, 0, 5, 0, 0, 0, 0};
  std::span<const uint8_t> bytes(record);

  // Act
  const auto it = sequence_.create_read_iterator(bytes);

  // Assert
  EXPECT_EQ(BareBones::StreamVByte::DecodeIteratorSentinel{}, it);
  EXPECT_TRUE(bytes.empty());
}

TEST_F(CompactSequenceCreateIteratorFixture, ReadsZeroValuesWithoutDecoderPadding) {
  // Arrange
  const std::array<uint8_t, 9> record{1, 0, 0, 0, 4, 0, 0, 0, 0};
  std::span<const uint8_t> bytes(record);

  // Act
  const auto it = sequence_.create_read_iterator(bytes);

  // Assert
  EXPECT_THAT(std::ranges::subrange(it, sequence_.end()), testing::ElementsAre(0, 0, 0, 0));
  EXPECT_TRUE(bytes.empty());
}

TEST_F(CompactSequenceCreateIteratorFixture, ReadsMixedWidthValuesWithoutDecoderPadding) {
  // Arrange
  // The body contains one key byte and seven value bytes for 0, 65536, 256, and 255.
  const std::array<uint8_t, 16> record{8, 0, 0, 0, 4, 0, 0, 0, 0x6C, 0, 0, 1, 0, 0, 1, 0xFF};
  std::span<const uint8_t> bytes(record);

  // Act
  const auto it = sequence_.create_read_iterator(bytes);

  // Assert
  EXPECT_THAT(std::ranges::subrange(it, sequence_.end()), testing::ElementsAre(0, 65536, 256, 255));
  EXPECT_TRUE(bytes.empty());
}

class CompactSequenceEightValueBlockFixture : public testing::Test {
 protected:
  CompactSequence<BareBones::StreamVByte::Codec0124, BareBones::MemoryWithItemCount, 8> sequence_;
};

TEST_F(CompactSequenceEightValueBlockFixture, ReadsFullZeroBlockWithoutNextBlockKey) {
  // Arrange
  const std::array<uint8_t, 10> record{2, 0, 0, 0, 8, 0, 0, 0, 0, 0};
  std::span<const uint8_t> bytes(record);

  // Act
  const auto iterator = sequence_.create_read_iterator(bytes);

  // Assert
  EXPECT_THAT(std::ranges::subrange(iterator, sequence_.end()), testing::ElementsAre(0, 0, 0, 0, 0, 0, 0, 0));
  EXPECT_TRUE(bytes.empty());
}

TEST_F(CompactSequenceEightValueBlockFixture, ReadsPartialZeroBlockWithUnusedKeyByte) {
  // Arrange
  const std::array<uint8_t, 10> record{2, 0, 0, 0, 4, 0, 0, 0, 0, 0};
  std::span<const uint8_t> bytes(record);

  // Act
  const auto iterator = sequence_.create_read_iterator(bytes);

  // Assert
  EXPECT_THAT(std::ranges::subrange(iterator, sequence_.end()), testing::ElementsAre(0, 0, 0, 0));
  EXPECT_TRUE(bytes.empty());
}

TEST_F(CompactSequenceEightValueBlockFixture, ReadsMixedWidthsAcrossKeyAndBlockBoundaries) {
  // Arrange
  const std::array values{0U, 65536U, 1U, 16777216U, 255U, 4294967295U, 256U, 16777215U, 65535U};
  std::ranges::copy(values, std::back_inserter(sequence_));
  BareBones::ShrinkedToFitOStringStream stream;
  sequence_.write_to(stream);
  auto bytes = stream.span<const uint8_t>();

  // Act
  const auto iterator = sequence_.create_read_iterator(bytes);

  // Assert
  EXPECT_THAT(std::ranges::subrange(iterator, sequence_.end()), testing::ElementsAreArray(values));
  EXPECT_TRUE(bytes.empty());
}

class CompactSequenceCreateIteratorParamFixture : public ::testing::TestWithParam<IotaCase> {
 protected:
  CompactSequence<BareBones::StreamVByte::Codec0124, BareBones::MemoryWithItemCount, 4> sequence_;
  BareBones::ShrinkedToFitOStringStream stream_;
};

TEST_P(CompactSequenceCreateIteratorParamFixture, ReadsValuesAndConsumesRecord) {
  // Arrange
  std::ranges::copy(GetParam().values, std::back_insert_iterator(sequence_));
  sequence_.write_to(stream_);
  auto buffer = stream_.span<const uint8_t>();

  // Act
  const auto it = sequence_.create_read_iterator(buffer);

  // Assert
  EXPECT_TRUE(std::ranges::equal(GetParam().values, std::ranges::subrange(it, sequence_.end())));
  EXPECT_TRUE(buffer.empty());
}

INSTANTIATE_TEST_SUITE_P(Cases, CompactSequenceCreateIteratorParamFixture, kIotaCases, kIotaCaseName);

template <class Codec>
class StreamVByteSequenceValuesFixture : public testing::Test {
 protected:
  using SequenceType = Sequence<Codec>;

  SequenceType sequence_;

  void fill_sequence(std::span<const uint32_t> values) { std::ranges::copy(values, std::back_inserter(sequence_)); }
};

using StreamVByte32BitCodecs = testing::Types<BareBones::StreamVByte::Codec0124, BareBones::StreamVByte::Codec1234>;
TYPED_TEST_SUITE(StreamVByteSequenceValuesFixture, StreamVByte32BitCodecs);

constexpr std::array<uint32_t, 9> kMixedWidthValues{0, 65536, 1, 16777216, 255, 4294967295, 256, 16777215, 65535};
constexpr std::array<uint32_t, 5> kZeroValues{0, 0, 0, 0, 0};

TYPED_TEST(StreamVByteSequenceValuesFixture, DefaultSequenceIsEmpty) {
  // Arrange

  // Act
  const auto values = this->sequence_.view();

  // Assert
  EXPECT_TRUE(values.empty());
}

TYPED_TEST(StreamVByteSequenceValuesFixture, PreservesZeroValuesAcrossKeyBoundary) {
  // Arrange

  // Act
  this->fill_sequence(kZeroValues);

  // Assert
  EXPECT_THAT(this->sequence_.view(), testing::ElementsAreArray(kZeroValues));
}

TYPED_TEST(StreamVByteSequenceValuesFixture, PreservesValuesAtEncodingWidthBoundaries) {
  // Arrange

  // Act
  this->fill_sequence(kMixedWidthValues);

  // Assert
  EXPECT_THAT(this->sequence_.view(), testing::ElementsAreArray(kMixedWidthValues));
}

TYPED_TEST(StreamVByteSequenceValuesFixture, StreamRoundTripPreservesEmptySequenceAndSaveSize) {
  // Arrange
  typename TestFixture::SequenceType restored;
  std::stringstream stream;

  // Act
  stream << this->sequence_;
  stream >> restored;

  // Assert
  EXPECT_EQ(this->sequence_.save_size(), stream.str().size());
  EXPECT_TRUE(restored.view().empty());
}

TYPED_TEST(StreamVByteSequenceValuesFixture, StreamRoundTripPreservesZeroValuesAndSaveSize) {
  // Arrange
  this->fill_sequence(kZeroValues);
  typename TestFixture::SequenceType restored;
  std::stringstream stream;

  // Act
  stream << this->sequence_;
  stream >> restored;

  // Assert
  EXPECT_EQ(this->sequence_.save_size(), stream.str().size());
  EXPECT_THAT(restored.view(), testing::ElementsAreArray(kZeroValues));
}

TYPED_TEST(StreamVByteSequenceValuesFixture, StreamRoundTripPreservesMixedWidthValuesAndSaveSize) {
  // Arrange
  this->fill_sequence(kMixedWidthValues);
  typename TestFixture::SequenceType restored;
  std::stringstream stream;

  // Act
  stream << this->sequence_;
  stream >> restored;

  // Assert
  EXPECT_EQ(this->sequence_.save_size(), stream.str().size());
  EXPECT_THAT(restored.view(), testing::ElementsAreArray(kMixedWidthValues));
}

template <class Codec>
class StreamVByteCompactValuesFixture : public testing::Test {
 protected:
  CompactSequence<Codec, BareBones::MemoryWithItemCount, 8> sequence_;
};

using CompactCodecTypes = testing::
    Types<BareBones::StreamVByte::Codec0124, BareBones::StreamVByte::Codec1234, BareBones::StreamVByte::Codec1238, BareBones::StreamVByte::Codec1238Mostly1>;
TYPED_TEST_SUITE(StreamVByteCompactValuesFixture, CompactCodecTypes);

TYPED_TEST(StreamVByteCompactValuesFixture, IteratorCopiesAdvanceIndependentlyAcrossKeysAndBlocks) {
  // Arrange
  const std::array values{0U, 65536U, 1U, 16777216U, 255U, 4294967295U, 256U, 16777215U, 65535U};
  std::ranges::copy(values, std::back_inserter(this->sequence_));
  auto first = this->sequence_.begin();
  auto copy = first;

  // Act
  std::ranges::advance(first, 4);
  const auto previous = copy++;
  std::ranges::advance(copy, 7);

  // Assert
  EXPECT_EQ(0U, *previous);
  EXPECT_EQ(255U, *first);
  EXPECT_EQ(65535U, *copy);
  EXPECT_NE(first, copy);
}

template <class Store>
class CompactSequenceExactBoundaryFixture : public testing::Test {
 protected:
  using Value = typename Store::value_type;
  static constexpr uint32_t kBlockValues = Store::kMaxKeySize * 4;
  Store sequence_;
  std::vector<Value> expected_;
  std::unique_ptr<uint8_t[]> record_;
  size_t record_size_{};

  void fill_full_width_block() {
    expected_.assign(kBlockValues, std::numeric_limits<Value>::max());
    std::ranges::copy(expected_, std::back_inserter(sequence_));
  }

  void make_unpadded_record(uint32_t count) {
    expected_.assign(count, std::numeric_limits<Value>::max());
    expected_.back() = 1;
    std::ranges::copy(expected_, std::back_inserter(sequence_));
    BareBones::ShrinkedToFitOStringStream stream;
    sequence_.write_to(stream);

    // Trim native padding to expose a wide read of the final one-byte value.
    const uint32_t body_size = Store::kMaxKeySize + (count - 1) * static_cast<uint32_t>(sizeof(Value)) + 1;
    record_size_ = 2 * sizeof(uint32_t) + body_size;
    record_ = std::make_unique<uint8_t[]>(record_size_);
    std::memcpy(record_.get(), stream.span<const uint8_t>().data(), record_size_);
    std::memcpy(record_.get(), &body_size, sizeof(body_size));
  }

  std::span<const uint8_t> record() const { return {record_.get(), record_size_}; }
};

template <class Codec, uint32_t Count>
using ExactBoundaryStore = CompactSequence<Codec, BareBones::MemoryWithItemCount, Count>;

using ExactBoundaryStores = testing::Types<ExactBoundaryStore<BareBones::StreamVByte::Codec0124, 4>,
                                           ExactBoundaryStore<BareBones::StreamVByte::Codec0124, 8>,
                                           ExactBoundaryStore<BareBones::StreamVByte::Codec0124, 1024>,
                                           ExactBoundaryStore<BareBones::StreamVByte::Codec1238, 4>,
                                           ExactBoundaryStore<BareBones::StreamVByte::Codec1238, 8>,
                                           ExactBoundaryStore<BareBones::StreamVByte::Codec1238, 1024>>;
TYPED_TEST_SUITE(CompactSequenceExactBoundaryFixture, ExactBoundaryStores);

TYPED_TEST(CompactSequenceExactBoundaryFixture, ReadsMaximumWidthOwnedBlockToEnd) {
  // Arrange
  this->fill_full_width_block();

  // Act
  const auto values = this->sequence_.view();

  // Assert
  EXPECT_THAT(values, testing::ElementsAreArray(this->expected_));
}

TYPED_TEST(CompactSequenceExactBoundaryFixture, ReadsFullExternalBlockWithNarrowTailWithoutPadding) {
  // Arrange
  this->make_unpadded_record(TestFixture::kBlockValues);
  auto bytes = this->record();

  // Act
  const auto values = TypeParam::read_view(bytes);

  // Assert
  EXPECT_THAT(values, testing::ElementsAreArray(this->expected_));
  EXPECT_TRUE(bytes.empty());
}

TYPED_TEST(CompactSequenceExactBoundaryFixture, ReadsPartialExternalBlockWithNarrowTailWithoutPadding) {
  // Arrange
  this->make_unpadded_record(TestFixture::kBlockValues - 1);
  auto bytes = this->record();

  // Act
  const auto values = TypeParam::read_view(bytes);

  // Assert
  EXPECT_THAT(values, testing::ElementsAreArray(this->expected_));
  EXPECT_TRUE(bytes.empty());
}

}  // namespace
