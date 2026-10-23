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

  // Act
  const auto view = this->sequence_.view();

  // Assert
  EXPECT_THAT(view, testing::ElementsAre());
  EXPECT_TRUE(this->sequence_.is_flushed());
}

TYPED_TEST(RleSequenceFixture, AppendMakesOneValueReadable) {
  // Arrange

  // Act
  this->sequence_.push_back(7);

  // Assert
  EXPECT_THAT(this->sequence_, testing::ElementsAre(7));
}

TYPED_TEST(RleSequenceFixture, AppendPreservesRepeatedValues) {
  // Arrange

  // Act
  this->sequence_.push_back(7);
  this->sequence_.push_back(7);
  this->sequence_.push_back(7);

  // Assert
  EXPECT_THAT(this->sequence_, testing::ElementsAre(7, 7, 7));
}

TYPED_TEST(RleSequenceFixture, AppendPreservesEarlierValues) {
  // Arrange
  this->sequence_.push_back(1);
  this->sequence_.push_back(2);

  // Act
  this->sequence_.push_back(4);

  // Assert
  EXPECT_THAT(this->sequence_, testing::ElementsAre(1, 2, 4));
}

TYPED_TEST(RleSequenceFixture, ReadingDoesNotFlushPendingValue) {
  // Arrange
  this->sequence_.push_back(7);

  // Act
  const auto view = this->sequence_.view();

  // Assert
  EXPECT_THAT(view, testing::ElementsAre(7));
  EXPECT_FALSE(this->sequence_.is_flushed());
  EXPECT_EQ(0U, this->sequence_.store().size());
}

TYPED_TEST(RleSequenceFixture, FlushPreservesValues) {
  // Arrange
  this->sequence_.push_back(1);
  this->sequence_.push_back(2);

  // Act
  this->sequence_.flush();

  // Assert
  EXPECT_TRUE(this->sequence_.is_flushed());
  EXPECT_THAT(this->sequence_, testing::ElementsAre(1, 2));
}

TYPED_TEST(RleSequenceFixture, FlushingTwicePreservesValues) {
  // Arrange
  this->sequence_.push_back(1);
  this->sequence_.flush();

  // Act
  this->sequence_.flush();

  // Assert
  EXPECT_TRUE(this->sequence_.is_flushed());
  EXPECT_THAT(this->sequence_, testing::ElementsAre(1));
}

TYPED_TEST(RleSequenceFixture, AppendAfterFlushPreservesEarlierValues) {
  // Arrange
  this->sequence_.push_back(1);
  this->sequence_.flush();

  // Act
  this->sequence_.push_back(2);

  // Assert
  EXPECT_THAT(this->sequence_, testing::ElementsAre(1, 2));
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

  // Act
  auto moved = std::move(this->sequence_);

  // Assert
  EXPECT_THAT(moved, testing::ElementsAre(1, 2, 4));
  EXPECT_THAT(this->sequence_, testing::ElementsAre());
}

TYPED_TEST(RleSequenceFixture, MovedFromSequenceCanBeReused) {
  // Arrange
  this->sequence_.push_back(1);
  this->sequence_.push_back(2);
  this->sequence_.push_back(4);
  auto moved = std::move(this->sequence_);

  // Act
  this->sequence_.push_back(0);

  // Assert
  EXPECT_THAT(this->sequence_, testing::ElementsAre(0));
  EXPECT_THAT(moved, testing::ElementsAre(1, 2, 4));
}

TYPED_TEST(RleSequenceFixture, ClearRemovesClosedAndPendingRuns) {
  // Arrange
  this->sequence_.push_back(1);
  this->sequence_.push_back(2);
  this->sequence_.push_back(4);

  // Act
  this->sequence_.clear();

  // Assert
  EXPECT_THAT(this->sequence_, testing::ElementsAre());
  EXPECT_TRUE(this->sequence_.is_flushed());
}

TYPED_TEST(RleSequenceFixture, AppendAfterClearResetsCoding) {
  // Arrange
  this->sequence_.push_back(1);
  this->sequence_.clear();

  // Act
  this->sequence_.push_back(0);

  // Assert
  EXPECT_THAT(this->sequence_, testing::ElementsAre(0));
}

TEST(RleSequenceViewTest, IdentityProgressionRepeatsOneValue) {
  // Arrange
  BareBones::RleSequence<BareBones::coding::Identity> sequence;
  const std::array<uint32_t, 3> input_values{7, 7, 7};
  std::ranges::copy(input_values, std::back_inserter(sequence));

  // Act
  const auto runs = sequence.progressions();

  // Assert
  EXPECT_THAT(runs, testing::ElementsAre(BareBones::ArithmeticProgression(7, 0, 3)));
  EXPECT_THAT(sequence, testing::ElementsAre(7, 7, 7));
}

TEST(RleSequenceViewTest, DeltaProgressionUsesTheDifferenceAsItsStep) {
  // Arrange
  BareBones::RleSequence<BareBones::coding::Delta> sequence;
  const std::array<uint32_t, 3> input_values{1, 2, 3};
  std::ranges::copy(input_values, std::back_inserter(sequence));

  // Act
  const auto runs = sequence.progressions();

  // Assert
  EXPECT_THAT(runs, testing::ElementsAre(BareBones::ArithmeticProgression(1, 1, 3)));
  EXPECT_THAT(sequence, testing::ElementsAre(1, 2, 3));
}

TEST(RleSequenceViewTest, DifferentDifferencesStartNewProgressions) {
  // Arrange
  BareBones::RleSequence<BareBones::coding::Delta> sequence;
  const std::array<uint32_t, 4> input_values{1, 2, 4, 6};
  std::ranges::copy(input_values, std::back_inserter(sequence));

  // Act
  const auto runs = sequence.progressions();

  // Assert
  EXPECT_THAT(runs, testing::ElementsAre(BareBones::ArithmeticProgression(1, 1, 2), BareBones::ArithmeticProgression(4, 2, 2)));
  EXPECT_THAT(sequence, testing::ElementsAre(1, 2, 4, 6));
}

TEST(RleSequenceViewTest, ZigZagProgressionsAllowNegativeSteps) {
  // Arrange
  BareBones::RleSequence<BareBones::coding::ZigZagDelta> sequence;
  const std::array<uint32_t, 4> input_values{10, 8, 6, 4};
  std::ranges::copy(input_values, std::back_inserter(sequence));

  // Act
  const auto runs = sequence.progressions();

  // Assert
  EXPECT_THAT(runs, testing::ElementsAre(BareBones::ArithmeticProgression(10, 0, 1), BareBones::ArithmeticProgression(8, -2, 3)));
  EXPECT_THAT(sequence, testing::ElementsAre(10, 8, 6, 4));
}

TEST(DeltaDeltaZigZagSequenceTest, DefaultSequenceIsEmpty) {
  // Arrange
  BareBones::DeltaDeltaZigZagSequence sequence;

  // Act
  const auto view = sequence.view();

  // Assert
  EXPECT_THAT(view, testing::ElementsAre());
}

TEST(DeltaDeltaZigZagSequenceTest, AppendMakesOneValueReadable) {
  // Arrange
  BareBones::DeltaDeltaZigZagSequence sequence;

  // Act
  sequence.push_back(10);

  // Assert
  EXPECT_THAT(sequence, testing::ElementsAre(10));
}

TEST(DeltaDeltaZigZagSequenceTest, AppendPreservesChangingAndNegativeDeltas) {
  // Arrange
  BareBones::DeltaDeltaZigZagSequence sequence;
  const std::array<uint32_t, 4> input{10, 20, 40, 30};

  // Act
  std::ranges::copy(input, std::back_inserter(sequence));

  // Assert
  EXPECT_THAT(sequence, testing::ElementsAre(10, 20, 40, 30));
}

TEST(EncodedSequenceReadTest, ReadsOneCompactRecord) {
  // Arrange
  BareBones::RleSequence<BareBones::coding::Identity, CompactStore> sequence;
  sequence.push_back(1);
  sequence.flush();
  BareBones::ShrinkedToFitOStringStream out;
  sequence.store().write_to(out);
  auto input = out.span<const uint8_t>();

  // Act
  const auto view = decltype(sequence)::read_view(input);

  // Assert
  EXPECT_THAT(view, testing::ElementsAre(1));
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
  BareBones::ShrinkedToFitOStringStream out;
  lengths.store().write_to(out);
  ids.store().write_to(out);
  auto input = out.span<const uint8_t>();

  // Act
  const auto read_lengths = BareBones::DeltaDeltaZigZagSequence::read_view(input);
  const auto read_ids = decltype(ids)::read_view(input);

  // Assert
  EXPECT_THAT(read_lengths, testing::ElementsAre(10, 20));
  EXPECT_THAT(read_ids, testing::ElementsAre(1, 2));
  EXPECT_TRUE(input.empty());
}

class RleLimitFixture : public testing::Test {
 protected:
  // Byte-sized counters exercise the run limit without billions of appends.
  BareBones::coding::detail::RunEncoder<std::vector<uint8_t>> encoder_;
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

  // Act
  append(7, 255);

  // Assert
  EXPECT_THAT(words_, testing::ElementsAre(7, 254));
  EXPECT_TRUE(encoder_.empty());
}

TEST_F(RleLimitFixture, FlushingMaximumRunDoesNotWriteItAgain) {
  // Arrange
  append(7, 255);

  // Act
  flush();

  // Assert
  EXPECT_THAT(words_, testing::ElementsAre(7, 254));
}

TEST_F(RleLimitFixture, SameValueAfterMaximumRunStartsANewRun) {
  // Arrange
  append(7, 255);

  // Act
  append(7, 1);
  flush();

  // Assert
  EXPECT_THAT(words_, testing::ElementsAre(7, 254, 7, 0));
}

TEST_F(RleLimitFixture, DifferentValueAfterMaximumRunStartsANewRun) {
  // Arrange
  append(7, 255);

  // Act
  append(9, 1);
  flush();

  // Assert
  EXPECT_THAT(words_, testing::ElementsAre(7, 254, 9, 0));
}

TEST(RleDecoderTest, MissingFinalCountDecodesSingleton) {
  // Arrange
  const std::array<uint32_t, 1> words{7};

  // Act
  const BareBones::RleView<BareBones::coding::Identity, std::span<const uint32_t>> view{words};

  // Assert
  EXPECT_THAT(view, testing::ElementsAre(7));
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
  const std::array<uint32_t, 8> input{0U, 2147483647U, 2147483648U, 4294967295U, 0U, 1U, 4294967295U, 2147483647U};

  // Act
  std::ranges::copy(input, std::back_inserter(this->sequence_));
  this->sequence_.flush();

  // Assert
  EXPECT_THAT(this->sequence_, testing::ElementsAre(0U, 2147483647U, 2147483648U, 4294967295U, 0U, 1U, 4294967295U, 2147483647U));
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
