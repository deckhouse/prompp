#include <array>
#include <iterator>
#include <limits>
#include <optional>
#include <random>
#include <span>
#include <sstream>
#include <string>
#include <vector>

#include <ranges>

#include <gtest/gtest.h>

#include "bare_bones/stream_v_byte.h"

namespace {

using BareBones::SharedPtrControlBlockWithItemCount;
using BareBones::StreamVByte::CompactSequence;
using BareBones::StreamVByte::Sequence;

const std::vector<uint64_t> ETALONS_BOUNDARY_VALUES = {1,        255,        256,        65535,         65536,         16777215,
                                                       16777216, 4294967295, 4294967296, 1099511627775, 1099511627776, 9223372036854775807};

template <class T>
class SequenceFixture : public testing::Test {};

using SequenceCodecTypes = testing::Types<Sequence<BareBones::StreamVByte::Codec1238>, Sequence<BareBones::StreamVByte::Codec1238Mostly1>>;
TYPED_TEST_SUITE(SequenceFixture, SequenceCodecTypes);

TYPED_TEST(SequenceFixture, Codec64) {
  TypeParam outcomes;

  for (auto& etalon : ETALONS_BOUNDARY_VALUES) {
    outcomes.push_back(etalon);
  }

  EXPECT_TRUE(std::ranges::equal(ETALONS_BOUNDARY_VALUES, outcomes));
}

template <class T>
class Container : public testing::Test {};

using ContainerCodecTypes = testing::Types<std::vector<uint8_t>, BareBones::Vector<uint8_t>>;

TYPED_TEST_SUITE(Container, ContainerCodecTypes);

TYPED_TEST(Container, Codec64) {
  TypeParam outcomes;

  auto out = BareBones::StreamVByte::back_inserter<BareBones::StreamVByte::Codec1238>(outcomes, ETALONS_BOUNDARY_VALUES.size());
  for (auto& etalon : ETALONS_BOUNDARY_VALUES) {
    *out++ = etalon;
  }

  auto [outcomes_b_i, outcomes_e_i] = BareBones::StreamVByte::decoder<BareBones::StreamVByte::Codec1238>(outcomes.begin(), ETALONS_BOUNDARY_VALUES.size());

  EXPECT_TRUE(std::ranges::equal(std::begin(ETALONS_BOUNDARY_VALUES), std::end(ETALONS_BOUNDARY_VALUES), outcomes_b_i, outcomes_e_i));
}

class SequenceIotaFixture : public ::testing::TestWithParam<std::ranges::iota_view<uint32_t, uint32_t>> {
 protected:
  Sequence<BareBones::StreamVByte::Codec0124, 4> sequence_;
};

TEST_P(SequenceIotaFixture, TestIota) {
  // Arrange

  // Act
  std::ranges::copy(GetParam(), std::back_insert_iterator(sequence_));

  // Assert
  EXPECT_TRUE(std::ranges::equal(GetParam(), sequence_));
}

class CompactSequenceIotaFixture : public ::testing::TestWithParam<std::ranges::iota_view<uint32_t, uint32_t>> {
 protected:
  CompactSequence<BareBones::StreamVByte::Codec0124, BareBones::MemoryWithItemCount, 4> sequence_;
};

TEST_P(CompactSequenceIotaFixture, TestIota) {
  // Arrange

  // Act
  std::ranges::copy(GetParam(), std::back_insert_iterator(sequence_));

  // Assert
  EXPECT_TRUE(std::ranges::equal(GetParam(), sequence_));
}

const auto kIotaCases = testing::Values(std::views::iota(0U, 0U),
                                        std::views::iota(0U, 3U),
                                        std::views::iota(0U, 4U),
                                        std::views::iota(0U, 5U),
                                        std::views::iota(0U, 8U),
                                        std::views::iota(0U, 9U),
                                        std::views::iota(std::numeric_limits<uint32_t>::max() - 4, std::numeric_limits<uint32_t>::max()));

INSTANTIATE_TEST_SUITE_P(Cases, SequenceIotaFixture, kIotaCases);
INSTANTIATE_TEST_SUITE_P(Cases, CompactSequenceIotaFixture, kIotaCases);

class ReadonlyCompactSequence : public testing::Test {
 protected:
  template <class T>
  using SharedMemory = BareBones::SharedMemory<T, SharedPtrControlBlockWithItemCount, BareBones::DefaultReallocator>;

  template <class T>
  using SharedSpan = BareBones::SharedSpan<T, SharedPtrControlBlockWithItemCount, BareBones::DefaultReallocator>;

  using Codec = BareBones::StreamVByte::Codec0124;

  using CompactSequenceSnapshot = CompactSequence<Codec, SharedSpan, 4>;

  CompactSequence<Codec, SharedMemory, 4> sequence_;
};

TEST_F(ReadonlyCompactSequence, Test) {
  // Arrange
  static constexpr std::array kValues{0U, 1U, 2U, 3U};

  std::ranges::copy(kValues, std::back_inserter(sequence_));

  // Act
  CompactSequenceSnapshot snapshot(sequence_);

  // Assert
  EXPECT_TRUE(std::ranges::equal(kValues, snapshot));
}

struct CompactSequenceAppendCase {
  const char* name;
  std::vector<uint32_t> initial_values;
  uint32_t value;
  std::vector<uint32_t> expected_values;
};

class CompactSequencePublicationFixture : public testing::TestWithParam<CompactSequenceAppendCase> {
  using Codec = BareBones::StreamVByte::Codec0124;

  class ObservedCodec : public Codec {
   public:
    static uint8_t encode(uint32_t value, uint8_t*& iterator) noexcept {
      const auto code = Codec::encode(value, iterator);
      fixture_->record_encoded_prefix();
      return code;
    }
  };

  class ObservedControlBlock : public SharedPtrControlBlockWithItemCount {
   public:
    void set_items_count(ItemCounter count) noexcept {
      fixture_->record_publication(count);
      SharedPtrControlBlockWithItemCount::set_items_count(count);
    }
  };

  template <class T>
  using SharedMemory = BareBones::SharedMemory<T, ObservedControlBlock, BareBones::DefaultReallocator>;

  using Sequence = CompactSequence<ObservedCodec, SharedMemory, 8>;

 protected:
  struct Observation {
    uint32_t size;
    std::vector<uint32_t> values;
  };

  Sequence sequence_;
  std::optional<Observation> encoded_prefix_;
  std::optional<Observation> publication_;
  uint32_t requested_size_{};

  void fill_sequence() {
    std::ranges::copy(GetParam().initial_values, std::back_inserter(sequence_));
    encoded_prefix_.reset();
    publication_.reset();
    requested_size_ = 0;
  }

 private:
  static inline CompactSequencePublicationFixture* fixture_{};

  void SetUp() override { fixture_ = this; }
  void TearDown() override { fixture_ = nullptr; }

  std::vector<uint32_t> read_prefix(uint32_t size, size_t available) const {
    std::vector<uint32_t> values;
    // Limit observations to the values supplied by the fixture.
    const auto iterator = Sequence::decode_iterator(sequence_.buffer().begin(), std::min(size, static_cast<uint32_t>(available)));
    std::ranges::copy(iterator, Sequence::end(), std::back_inserter(values));
    return values;
  }

  void record_encoded_prefix() {
    const auto size = sequence_.size();
    encoded_prefix_ = Observation{.size = size, .values = read_prefix(size, GetParam().initial_values.size())};
  }

  void record_publication(uint32_t new_size) {
    requested_size_ = new_size;
    publication_ = Observation{.size = sequence_.size(), .values = read_prefix(new_size, GetParam().expected_values.size())};
  }
};

TEST_P(CompactSequencePublicationFixture, KeepsSizeAndPrefixDuringEncoding) {
  // Arrange
  fill_sequence();

  // Act
  sequence_.push_back(GetParam().value);

  // Assert
  ASSERT_TRUE(encoded_prefix_.has_value());
  EXPECT_EQ(GetParam().initial_values.size(), encoded_prefix_->size);
  EXPECT_EQ(GetParam().initial_values, encoded_prefix_->values);
}

TEST_P(CompactSequencePublicationFixture, PublishesSizeAfterValueAndKey) {
  // Arrange
  fill_sequence();

  // Act
  sequence_.push_back(GetParam().value);

  // Assert
  ASSERT_TRUE(publication_.has_value());
  EXPECT_EQ(GetParam().initial_values.size(), publication_->size);
  EXPECT_EQ(GetParam().expected_values.size(), requested_size_);
  EXPECT_EQ(GetParam().expected_values, publication_->values);
  EXPECT_EQ(GetParam().expected_values.size(), sequence_.size());
  EXPECT_TRUE(std::ranges::equal(GetParam().expected_values, sequence_));
}

INSTANTIATE_TEST_SUITE_P(
    Append,
    CompactSequencePublicationFixture,
    testing::Values(
        CompactSequenceAppendCase{.name = "Empty", .initial_values = {}, .value = 65536, .expected_values = {65536}},
        CompactSequenceAppendCase{.name = "ZeroValue", .initial_values = {1}, .value = 0, .expected_values = {1, 0}},
        CompactSequenceAppendCase{.name = "ZeroPrefix", .initial_values = {0}, .value = 1, .expected_values = {0, 1}},
        CompactSequenceAppendCase{.name = "OneByte", .initial_values = {1}, .value = 255, .expected_values = {1, 255}},
        CompactSequenceAppendCase{.name = "TwoBytes", .initial_values = {1}, .value = 256, .expected_values = {1, 256}},
        CompactSequenceAppendCase{.name = "FourBytes", .initial_values = {1}, .value = 65536, .expected_values = {1, 65536}},
        CompactSequenceAppendCase{.name = "ThirdKeySlot", .initial_values = {1, 256}, .value = 65536, .expected_values = {1, 256, 65536}},
        CompactSequenceAppendCase{.name = "LastKeySlot",
                                  .initial_values = {1, 256, 65536},
                                  .value = 4294967295U,
                                  .expected_values = {1, 256, 65536, 4294967295U}},
        CompactSequenceAppendCase{.name = "NextKeyByte", .initial_values = {1, 256, 65536, 0}, .value = 255, .expected_values = {1, 256, 65536, 0, 255}},
        CompactSequenceAppendCase{.name = "NextBlock",
                                  .initial_values = {1, 256, 65536, 0, 255, 65535, 0, 1},
                                  .value = 4294967295U,
                                  .expected_values = {1, 256, 65536, 0, 255, 65535, 0, 1, 4294967295U}}),
    [](const testing::TestParamInfo<CompactSequenceAppendCase>& info) { return info.param.name; });

class CompactSequenceCreateIteratorFixture : public testing::Test {
 protected:
  CompactSequence<BareBones::StreamVByte::Codec0124, BareBones::MemoryWithItemCount, 4> sequence_;
};

TEST_F(CompactSequenceCreateIteratorFixture, KeepsCapturedSizeAfterAppend) {
  // Arrange
  const std::array initial_values{1U, 256U};
  const std::array expected_values{1U, 256U, 65536U};
  std::ranges::copy(initial_values, std::back_inserter(sequence_));
  const auto iterator = sequence_.begin();

  // Act
  sequence_.push_back(65536);

  // Assert
  EXPECT_TRUE(std::ranges::equal(initial_values.begin(), initial_values.end(), iterator, sequence_.end()));
  EXPECT_TRUE(std::ranges::equal(expected_values, sequence_));
}

TEST_F(CompactSequenceCreateIteratorFixture, CreateReadIteratorNotEnoughBytes) {
  // Arrange
  std::array<uint8_t, 6> data{};
  std::span<const uint8_t> buffer(data);

  // Act
  const auto it = sequence_.create_read_iterator(buffer);

  // Assert
  EXPECT_EQ(it, BareBones::StreamVByte::DecodeIteratorSentinel{});
  EXPECT_EQ(buffer.size(), data.size());
}

TEST_F(CompactSequenceCreateIteratorFixture, CreateReadIteratorNotEnoughSizeRead) {
  // Arrange
  std::array<uint32_t, 2> data{3, 0};
  std::span<const uint8_t> buffer(reinterpret_cast<const uint8_t*>(data.data()), data.size() * sizeof(uint32_t));

  // Act
  const auto it = sequence_.create_read_iterator(buffer);

  // Assert
  EXPECT_EQ(it, BareBones::StreamVByte::DecodeIteratorSentinel{});
  EXPECT_EQ(buffer.size(), 0);
}

class CompactSequenceCreateIteratorParamFixture : public ::testing::TestWithParam<std::ranges::iota_view<uint32_t, uint32_t>> {
 protected:
  CompactSequence<BareBones::StreamVByte::Codec0124, BareBones::MemoryWithItemCount, 4> sequence_;
  BareBones::ShrinkedToFitOStringStream stream_;
};

TEST_P(CompactSequenceCreateIteratorParamFixture, CreateReadIteratorValid) {
  // Arrange
  std::ranges::copy(GetParam(), std::back_insert_iterator(sequence_));
  sequence_.write_to(stream_);
  auto buffer = stream_.span<const uint8_t>();

  // Act
  const auto it = sequence_.create_read_iterator(buffer);

  // Assert
  EXPECT_TRUE(std::ranges::equal(it, BareBones::StreamVByte::DecodeIteratorSentinel{}, sequence_.begin(), sequence_.end()));
  EXPECT_EQ(buffer.size(), 0);
}

INSTANTIATE_TEST_SUITE_P(Cases, CompactSequenceCreateIteratorParamFixture, kIotaCases);

struct SequenceValuesCase {
  const char* name;
  std::vector<uint32_t> values;
};

template <class Codec>
class StreamVByteSequenceValuesFixture : public testing::TestWithParam<SequenceValuesCase> {
 protected:
  Sequence<Codec> sequence_;

  void fill_sequence() { std::ranges::copy(this->GetParam().values, std::back_inserter(sequence_)); }

 public:
  static auto cases() {
    std::mt19937 generator(42);
    std::vector<uint32_t> mixed_values(100);
    std::ranges::generate(mixed_values, [&generator] { return generator(); });
    return testing::Values(SequenceValuesCase{.name = "Empty", .values = {}}, SequenceValuesCase{.name = "Zeros", .values = std::vector<uint32_t>(100, 0)},
                           SequenceValuesCase{.name = "MixedValues", .values = std::move(mixed_values)});
  }
};

using StreamVByte0124ValuesFixture = StreamVByteSequenceValuesFixture<BareBones::StreamVByte::Codec0124>;
using StreamVByte1234ValuesFixture = StreamVByteSequenceValuesFixture<BareBones::StreamVByte::Codec1234>;

TEST_P(StreamVByte0124ValuesFixture, PreservesInsertionOrder) {
  // Arrange

  // Act
  fill_sequence();

  // Assert
  EXPECT_TRUE(std::ranges::equal(GetParam().values, sequence_));
}

TEST_P(StreamVByte0124ValuesFixture, StreamRoundTripPreservesValuesAndSaveSize) {
  // Arrange
  fill_sequence();
  Sequence<BareBones::StreamVByte::Codec0124> restored;
  std::stringstream stream;

  // Act
  stream << sequence_;
  stream >> restored;

  // Assert
  EXPECT_EQ(sequence_.save_size(), stream.str().size());
  EXPECT_TRUE(std::ranges::equal(GetParam().values, restored));
}

INSTANTIATE_TEST_SUITE_P(EmptyZerosAndMixedValues,
                         StreamVByte0124ValuesFixture,
                         StreamVByte0124ValuesFixture::cases(),
                         [](const testing::TestParamInfo<SequenceValuesCase>& info) { return info.param.name; });

TEST_P(StreamVByte1234ValuesFixture, PreservesInsertionOrder) {
  // Arrange

  // Act
  fill_sequence();

  // Assert
  EXPECT_TRUE(std::ranges::equal(GetParam().values, sequence_));
}

TEST_P(StreamVByte1234ValuesFixture, StreamRoundTripPreservesValuesAndSaveSize) {
  // Arrange
  fill_sequence();
  Sequence<BareBones::StreamVByte::Codec1234> restored;
  std::stringstream stream;

  // Act
  stream << sequence_;
  stream >> restored;

  // Assert
  EXPECT_EQ(sequence_.save_size(), stream.str().size());
  EXPECT_TRUE(std::ranges::equal(GetParam().values, restored));
}

INSTANTIATE_TEST_SUITE_P(EmptyZerosAndMixedValues,
                         StreamVByte1234ValuesFixture,
                         StreamVByte1234ValuesFixture::cases(),
                         [](const testing::TestParamInfo<SequenceValuesCase>& info) { return info.param.name; });

}  // namespace
