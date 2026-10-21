#include <array>
#include <limits>
#include <vector>

#include <gmock/gmock.h>

#include "bare_bones/encoding.h"

namespace {

class RleLimitFixture : public testing::Test {
 protected:
  using Backend = BareBones::Encoding::RLEBackend<std::vector<uint8_t>>;

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

  std::vector<uint8_t> values() const {
    auto begin = words_.begin();
    Backend::Decoder decoder(begin, words_.end(), encoder_);
    std::vector<uint8_t> values;
    while (!decoder.is_finished(begin, words_.end(), encoder_)) {
      values.push_back(decoder.decode(begin, words_.end(), encoder_));
      decoder.next(begin, words_.end(), encoder_);
    }
    return values;
  }

  Backend::Encoder encoder_;
  std::vector<uint8_t> words_;
};

TEST_F(RleLimitFixture, MaximumRunHasNoExtraOpenValue) {
  // Arrange
  const std::vector<uint8_t> expected(255, 7);

  // Act
  append(7, 255);

  // Assert
  EXPECT_EQ(expected, values());
  EXPECT_TRUE(encoder_.empty());
}

TEST_F(RleLimitFixture, FlushingMaximumRunDoesNotDuplicateValue) {
  // Arrange
  append(7, 255);
  const std::vector<uint8_t> expected(255, 7);

  // Act
  flush();

  // Assert
  EXPECT_EQ(expected, values());
  EXPECT_THAT(words_, testing::ElementsAre(7, 254));
}

TEST_F(RleLimitFixture, SameValueAfterMaximumRunStartsOneNewValue) {
  // Arrange
  append(7, 255);
  const std::vector<uint8_t> expected(256, 7);

  // Act
  append(7, 1);
  flush();

  // Assert
  EXPECT_EQ(expected, values());
}

TEST_F(RleLimitFixture, DifferentValueAfterMaximumRunStartsOneNewValue) {
  // Arrange
  append(7, 255);
  std::vector<uint8_t> expected(256, 7);
  expected.back() = 9;

  // Act
  append(9, 1);
  flush();

  // Assert
  EXPECT_EQ(expected, values());
}

TEST(RleDecoderTest, MissingFinalCountDecodesSingleton) {
  // Arrange
  using Backend = BareBones::Encoding::RLEBackend<std::vector<uint32_t>>;
  Backend::Encoder encoder;
  const std::vector<uint32_t> words{7};
  auto begin = words.begin();

  // Act
  Backend::Decoder decoder(begin, words.end(), encoder);
  const auto value = decoder.decode(begin, words.end(), encoder);
  decoder.next(begin, words.end(), encoder);

  // Assert
  EXPECT_EQ(7U, value);
  EXPECT_TRUE(decoder.is_finished(begin, words.end(), encoder));
}

using Store32 = BareBones::StreamVByte::Sequence<BareBones::StreamVByte::Codec0124Frequent0>;
using Store64 = BareBones::StreamVByte::Sequence<BareBones::StreamVByte::Codec1238>;
using SignedBoundarySequences = testing::Types<BareBones::EncodedSequence<BareBones::Encoding::DeltaZigZagRLE<Store32>>,
                                               BareBones::EncodedSequence<BareBones::Encoding::DeltaZigZagRLE<Store64>>,
                                               BareBones::EncodedSequence<BareBones::Encoding::DeltaDeltaZigZag<Store32>>,
                                               BareBones::EncodedSequence<BareBones::Encoding::DeltaDeltaZigZag<Store64>>>;

template <class Sequence>
class SignedBoundaryFixture : public testing::Test {};
TYPED_TEST_SUITE(SignedBoundaryFixture, SignedBoundarySequences);

TYPED_TEST(SignedBoundaryFixture, PreservesValuesAcrossSignedBoundaries) {
  // Arrange
  using Value = typename TypeParam::value_type;
  constexpr auto max = std::numeric_limits<Value>::max();
  const std::array<Value, 8> expected{0, max / 2, max / 2 + 1, max, 0, 1, max, max / 2};
  TypeParam sequence;

  // Act
  std::ranges::copy(expected, std::back_inserter(sequence));
  sequence.flush();

  // Assert
  EXPECT_THAT(sequence, testing::ElementsAreArray(expected));
}

}  // namespace
