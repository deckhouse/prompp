#include <array>
#include <vector>

#include <gmock/gmock.h>

#include "bare_bones/encoding.h"
#include "bare_bones/streams.h"

namespace {
using Store = BareBones::StreamVByte::Sequence<BareBones::StreamVByte::Codec0124Frequent0>;
using CompactStore = BareBones::StreamVByte::CompactSequence<BareBones::StreamVByte::Codec0124Frequent0>;
using ProductionSequences = testing::Types<BareBones::RleSequence<BareBones::coding::Identity, Store>,
                                           BareBones::RleSequence<BareBones::coding::Delta, Store>,
                                           BareBones::RleSequence<BareBones::coding::ZigZagDelta, Store>,
                                           BareBones::RleSequence<BareBones::coding::Identity, CompactStore>,
                                           BareBones::RleSequence<BareBones::coding::Delta, CompactStore>,
                                           BareBones::RleSequence<BareBones::coding::ZigZagDelta, CompactStore>>;
template <class T>
class RleSequenceFixture : public testing::Test {
 protected:
  T sequence_;
};
TYPED_TEST_SUITE(RleSequenceFixture, ProductionSequences);

TYPED_TEST(RleSequenceFixture, DefaultSequenceIsEmpty) {
  // Arrange
  const std::array<uint32_t, 0> expected{};

  // Act
  const auto view = this->sequence_.view();

  // Assert
  EXPECT_THAT(view, testing::ElementsAreArray(expected));
  EXPECT_TRUE(this->sequence_.is_flushed());
}

TYPED_TEST(RleSequenceFixture, AppendMakesOneValueReadable) {
  // Arrange
  const std::array<uint32_t, 1> expected{7};

  // Act
  this->sequence_.push_back(7);

  // Assert
  EXPECT_THAT(this->sequence_, testing::ElementsAreArray(expected));
}

TYPED_TEST(RleSequenceFixture, AppendPreservesRepeatedValues) {
  // Arrange
  const std::array<uint32_t, 3> expected{7, 7, 7};

  // Act
  this->sequence_.push_back(7);
  this->sequence_.push_back(7);
  this->sequence_.push_back(7);

  // Assert
  EXPECT_THAT(this->sequence_, testing::ElementsAreArray(expected));
}

TYPED_TEST(RleSequenceFixture, AppendPreservesEarlierValues) {
  // Arrange
  this->sequence_.push_back(1);
  this->sequence_.push_back(2);
  const std::array<uint32_t, 3> expected{1, 2, 4};

  // Act
  this->sequence_.push_back(4);

  // Assert
  EXPECT_THAT(this->sequence_, testing::ElementsAreArray(expected));
}

TYPED_TEST(RleSequenceFixture, ReadingDoesNotFlushPendingValue) {
  // Arrange
  this->sequence_.push_back(7);
  const std::array<uint32_t, 1> expected{7};

  // Act
  const auto view = this->sequence_.view();

  // Assert
  EXPECT_THAT(view, testing::ElementsAreArray(expected));
  EXPECT_FALSE(this->sequence_.is_flushed());
  EXPECT_EQ(0U, this->sequence_.store().size());
}

TYPED_TEST(RleSequenceFixture, FlushPreservesValues) {
  // Arrange
  this->sequence_.push_back(1);
  this->sequence_.push_back(2);
  const std::array<uint32_t, 2> expected{1, 2};

  // Act
  this->sequence_.flush();

  // Assert
  EXPECT_TRUE(this->sequence_.is_flushed());
  EXPECT_THAT(this->sequence_, testing::ElementsAreArray(expected));
}

TYPED_TEST(RleSequenceFixture, FlushingTwicePreservesValues) {
  // Arrange
  this->sequence_.push_back(1);
  this->sequence_.flush();
  const std::array<uint32_t, 1> expected{1};

  // Act
  this->sequence_.flush();

  // Assert
  EXPECT_TRUE(this->sequence_.is_flushed());
  EXPECT_THAT(this->sequence_, testing::ElementsAreArray(expected));
}

TYPED_TEST(RleSequenceFixture, AppendAfterFlushPreservesEarlierValues) {
  // Arrange
  this->sequence_.push_back(1);
  this->sequence_.flush();
  const std::array<uint32_t, 2> expected{1, 2};

  // Act
  this->sequence_.push_back(2);

  // Assert
  EXPECT_THAT(this->sequence_, testing::ElementsAreArray(expected));
}

TYPED_TEST(RleSequenceFixture, IteratorReadsPendingTailAfterTemporaryViewDies) {
  // Arrange
  this->sequence_.push_back(1);
  this->sequence_.push_back(2);
  this->sequence_.push_back(4);

  // Act
  auto it = this->sequence_.view().begin();
  ++it;
  ++it;

  // Assert
  EXPECT_EQ(4U, *it);
  static_assert(std::ranges::borrowed_range<typename TypeParam::View>);
  static_assert(std::ranges::input_range<typename TypeParam::View>);
}

TYPED_TEST(RleSequenceFixture, MovePreservesClosedAndPendingRuns) {
  // Arrange
  this->sequence_.push_back(1);
  this->sequence_.push_back(2);
  this->sequence_.push_back(4);
  const std::array<uint32_t, 3> expected{1, 2, 4};
  const std::array<uint32_t, 0> expected_source{};

  // Act
  auto moved = std::move(this->sequence_);

  // Assert
  EXPECT_THAT(moved, testing::ElementsAreArray(expected));
  EXPECT_THAT(this->sequence_, testing::ElementsAreArray(expected_source));
}

TYPED_TEST(RleSequenceFixture, MovedFromSequenceCanBeReused) {
  // Arrange
  this->sequence_.push_back(1);
  this->sequence_.push_back(2);
  this->sequence_.push_back(4);
  auto moved = std::move(this->sequence_);
  const std::array<uint32_t, 1> expected_source{0};
  const std::array<uint32_t, 3> expected_moved{1, 2, 4};

  // Act
  this->sequence_.push_back(0);

  // Assert
  EXPECT_THAT(this->sequence_, testing::ElementsAreArray(expected_source));
  EXPECT_THAT(moved, testing::ElementsAreArray(expected_moved));
}

TYPED_TEST(RleSequenceFixture, ClearRemovesClosedAndPendingRuns) {
  // Arrange
  this->sequence_.push_back(1);
  this->sequence_.push_back(2);
  this->sequence_.push_back(4);
  const std::array<uint32_t, 0> expected{};

  // Act
  this->sequence_.clear();

  // Assert
  EXPECT_THAT(this->sequence_, testing::ElementsAreArray(expected));
  EXPECT_TRUE(this->sequence_.is_flushed());
}

TYPED_TEST(RleSequenceFixture, AppendAfterClearResetsCoding) {
  // Arrange
  this->sequence_.push_back(1);
  this->sequence_.clear();
  const std::array<uint32_t, 1> expected{0};

  // Act
  this->sequence_.push_back(0);

  // Assert
  EXPECT_THAT(this->sequence_, testing::ElementsAreArray(expected));
}

TEST(RleSequenceViewTest, IdentityProgressionRepeatsOneValue) {
  // Arrange
  BareBones::RleSequence<BareBones::coding::Identity> sequence;
  const std::array<uint32_t, 3> expected_values{7, 7, 7};
  const std::array<BareBones::ArithmeticProgression, 1> expected_runs{{{7, 0, 3}}};
  std::ranges::copy(expected_values, std::back_inserter(sequence));

  // Act
  const auto runs = sequence.progressions();

  // Assert
  EXPECT_THAT(runs, testing::ElementsAreArray(expected_runs));
  EXPECT_THAT(sequence, testing::ElementsAreArray(expected_values));
}

TEST(RleSequenceViewTest, DeltaProgressionUsesTheDifferenceAsItsStep) {
  // Arrange
  BareBones::RleSequence<BareBones::coding::Delta> sequence;
  const std::array<uint32_t, 3> expected_values{1, 2, 3};
  const std::array<BareBones::ArithmeticProgression, 1> expected_runs{{{1, 1, 3}}};
  std::ranges::copy(expected_values, std::back_inserter(sequence));

  // Act
  const auto runs = sequence.progressions();

  // Assert
  EXPECT_THAT(runs, testing::ElementsAreArray(expected_runs));
  EXPECT_THAT(sequence, testing::ElementsAreArray(expected_values));
}

TEST(RleSequenceViewTest, DifferentDifferencesStartNewProgressions) {
  // Arrange
  BareBones::RleSequence<BareBones::coding::Delta> sequence;
  const std::array<uint32_t, 4> expected_values{1, 2, 4, 6};
  const std::array<BareBones::ArithmeticProgression, 2> expected_runs{{{1, 1, 2}, {4, 2, 2}}};
  std::ranges::copy(expected_values, std::back_inserter(sequence));

  // Act
  const auto runs = sequence.progressions();

  // Assert
  EXPECT_THAT(runs, testing::ElementsAreArray(expected_runs));
  EXPECT_THAT(sequence, testing::ElementsAreArray(expected_values));
}

TEST(RleSequenceViewTest, ZigZagProgressionsAllowNegativeSteps) {
  // Arrange
  BareBones::RleSequence<BareBones::coding::ZigZagDelta> sequence;
  const std::array<uint32_t, 4> expected_values{10, 8, 6, 4};
  const std::array<BareBones::ArithmeticProgression, 2> expected_runs{{{10, 0, 1}, {8, -2, 3}}};
  std::ranges::copy(expected_values, std::back_inserter(sequence));

  // Act
  const auto runs = sequence.progressions();

  // Assert
  EXPECT_THAT(runs, testing::ElementsAreArray(expected_runs));
  EXPECT_THAT(sequence, testing::ElementsAreArray(expected_values));
}

TEST(DeltaDeltaZigZagSequenceTest, DefaultSequenceIsEmpty) {
  // Arrange
  BareBones::DeltaDeltaZigZagSequence sequence;
  const std::array<uint32_t, 0> expected{};

  // Act
  const auto view = sequence.view();

  // Assert
  EXPECT_THAT(view, testing::ElementsAreArray(expected));
}

TEST(DeltaDeltaZigZagSequenceTest, AppendMakesOneValueReadable) {
  // Arrange
  BareBones::DeltaDeltaZigZagSequence sequence;
  const std::array<uint32_t, 1> expected{10};

  // Act
  sequence.push_back(10);

  // Assert
  EXPECT_THAT(sequence, testing::ElementsAreArray(expected));
}

TEST(DeltaDeltaZigZagSequenceTest, AppendPreservesChangingAndNegativeDeltas) {
  // Arrange
  BareBones::DeltaDeltaZigZagSequence sequence;
  const std::array<uint32_t, 4> expected{10, 20, 40, 30};

  // Act
  std::ranges::copy(expected, std::back_inserter(sequence));

  // Assert
  EXPECT_THAT(sequence, testing::ElementsAreArray(expected));
}

TEST(EncodedSequenceReadTest, ReadsOneCompactRecord) {
  // Arrange
  BareBones::RleSequence<BareBones::coding::Identity, CompactStore> sequence;
  sequence.push_back(1);
  sequence.flush();
  const std::array<uint32_t, 1> expected{1};
  BareBones::ShrinkedToFitOStringStream out;
  sequence.store().write_to(out);
  auto input = out.span<const uint8_t>();

  // Act
  const auto view = decltype(sequence)::read_view(input);

  // Assert
  EXPECT_THAT(view, testing::ElementsAreArray(expected));
  EXPECT_TRUE(input.empty());
}

TEST(EncodedSequenceReadTest, ConsumesConsecutiveCompactRecords) {
  // Arrange
  BareBones::DeltaDeltaZigZagSequence lengths;
  BareBones::RleSequence<BareBones::coding::Identity, CompactStore> ids;
  lengths.push_back(10);
  lengths.push_back(20);
  ids.push_back(1);
  ids.push_back(2);
  ids.flush();
  const std::array<uint32_t, 2> expected_lengths{10, 20};
  const std::array<uint32_t, 2> expected_ids{1, 2};
  BareBones::ShrinkedToFitOStringStream out;
  lengths.store().write_to(out);
  ids.store().write_to(out);
  auto input = out.span<const uint8_t>();

  // Act
  const auto read_lengths = BareBones::DeltaDeltaZigZagSequence::read_view(input);
  const auto read_ids = decltype(ids)::read_view(input);

  // Assert
  EXPECT_THAT(read_lengths, testing::ElementsAreArray(expected_lengths));
  EXPECT_THAT(read_ids, testing::ElementsAreArray(expected_ids));
  EXPECT_TRUE(input.empty());
}

class RleLimitFixture : public testing::Test {
 protected:
  // Byte-sized counters exercise the run limit without billions of appends.
  BareBones::encoding_detail::RunEncoder<std::vector<uint8_t>> encoder_;
  std::vector<uint8_t> words_;

  void append(uint8_t value, uint32_t count) {
    auto out = std::back_inserter(words_);
    for (uint32_t i = 0; i < count; ++i) {
      encoder_.encode(value, out);
    }
  }

  void flush() {
    auto out = std::back_inserter(words_);
    encoder_.flush(out);
  }
};

TEST_F(RleLimitFixture, MaximumRunIsClosedWithoutAnExtraPendingValue) {
  // Arrange
  const std::array<uint8_t, 2> expected{7, 254};

  // Act
  append(7, 255);

  // Assert
  EXPECT_THAT(words_, testing::ElementsAreArray(expected));
  EXPECT_TRUE(encoder_.empty());
}

TEST_F(RleLimitFixture, FlushingMaximumRunDoesNotWriteItAgain) {
  // Arrange
  append(7, 255);
  const std::array<uint8_t, 2> expected{7, 254};

  // Act
  flush();

  // Assert
  EXPECT_THAT(words_, testing::ElementsAreArray(expected));
}

TEST_F(RleLimitFixture, SameValueAfterMaximumRunStartsANewRun) {
  // Arrange
  append(7, 255);
  const std::array<uint8_t, 4> expected{7, 254, 7, 0};

  // Act
  append(7, 1);
  flush();

  // Assert
  EXPECT_THAT(words_, testing::ElementsAreArray(expected));
}

TEST_F(RleLimitFixture, DifferentValueAfterMaximumRunStartsANewRun) {
  // Arrange
  append(7, 255);
  const std::array<uint8_t, 4> expected{7, 254, 9, 0};

  // Act
  append(9, 1);
  flush();

  // Assert
  EXPECT_THAT(words_, testing::ElementsAreArray(expected));
}

TEST(RleDecoderTest, MissingFinalCountDecodesSingleton) {
  // Arrange
  const std::array<uint32_t, 1> words{7};
  const std::array<uint32_t, 1> expected{7};

  // Act
  const BareBones::RleView<BareBones::coding::Identity, std::span<const uint32_t>> view{words};

  // Assert
  EXPECT_THAT(view, testing::ElementsAreArray(expected));
}

using SignedBoundarySequences = testing::Types<BareBones::RleSequence<BareBones::coding::ZigZagDelta>, BareBones::DeltaDeltaZigZagSequence>;

template <class Sequence>
class SignedBoundaryFixture : public testing::Test {
 protected:
  Sequence sequence_;
};
TYPED_TEST_SUITE(SignedBoundaryFixture, SignedBoundarySequences);

TYPED_TEST(SignedBoundaryFixture, PreservesValuesAcrossSignedBoundaries) {
  // Arrange
  const std::array<uint32_t, 8> expected{0U, 2147483647U, 2147483648U, 4294967295U, 0U, 1U, 4294967295U, 2147483647U};

  // Act
  std::ranges::copy(expected, std::back_inserter(this->sequence_));
  this->sequence_.flush();

  // Assert
  EXPECT_THAT(this->sequence_, testing::ElementsAreArray(expected));
}

struct ZigZagWrappingCase {
  const char* name;
  std::vector<uint32_t> values;
  std::vector<BareBones::ArithmeticProgression> progressions;
};
class ZigZagWrappingFixture : public testing::TestWithParam<ZigZagWrappingCase> {
 protected:
  BareBones::RleSequence<BareBones::coding::ZigZagDelta> sequence_;

  void SetUp() override { std::ranges::copy(GetParam().values, std::back_inserter(sequence_)); }
};
TEST_P(ZigZagWrappingFixture, OpenRunPreservesValuesAcrossUnsignedBoundary) {
  // Arrange
  const auto& expected = GetParam().values;

  // Act
  const auto view = sequence_.view();

  // Assert
  ASSERT_TRUE(view.is_valid());
  EXPECT_THAT(view, testing::ElementsAreArray(expected));
}
TEST_P(ZigZagWrappingFixture, FlushedRunPreservesValuesAcrossUnsignedBoundary) {
  // Arrange
  const auto& expected = GetParam().values;

  // Act
  sequence_.flush();

  // Assert
  ASSERT_TRUE(sequence_.view().is_valid());
  EXPECT_THAT(sequence_, testing::ElementsAreArray(expected));
}
TEST_P(ZigZagWrappingFixture, ProgressionsStopAtUnsignedBoundary) {
  // Arrange
  const auto& expected = GetParam().progressions;

  // Act
  const auto progressions = sequence_.progressions();

  // Assert
  EXPECT_THAT(progressions, testing::ElementsAreArray(expected));
}
INSTANTIATE_TEST_SUITE_P(
    UnsignedWrap,
    ZigZagWrappingFixture,
    testing::Values(ZigZagWrappingCase{.name = "NegativeStep", .values = {1U, 0U, 4294967295U}, .progressions = {{1, 0, 1}, {0, 0, 1}, {4294967295U, 0, 1}}},
                    ZigZagWrappingCase{.name = "PositiveStep",
                                       .values = {2147483647U, 4294967294U, 2147483645U},
                                       .progressions = {{2147483647U, 2147483647, 2}, {2147483645U, 0, 1}}},
                    ZigZagWrappingCase{.name = "MinimumSignedStep",
                                       .values = {2147483648U, 0U, 2147483648U, 0U},
                                       .progressions = {{2147483648U, -2147483648LL, 2}, {2147483648U, -2147483648LL, 2}}},
                    ZigZagWrappingCase{.name = "NextRunAfterWrap",
                                       .values = {1U, 0U, 4294967295U, 10U},
                                       .progressions = {{1, 0, 1}, {0, 0, 1}, {4294967295U, 0, 1}, {10, 0, 1}}}),
    [](const testing::TestParamInfo<ZigZagWrappingCase>& info) { return info.param.name; });

}  // namespace
