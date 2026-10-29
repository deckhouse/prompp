#include <gtest/gtest.h>

#include <limits>
#include <utility>

#include "primitives/go_slice.h"

namespace {

using PromPP::Primitives::Go::BytesStream;
using PromPP::Primitives::Go::Slice;

class BytesStreamFixture : public testing::Test {
 protected:
  Slice<char> slice_;
  BytesStream stream_{&slice_};
};

TEST_F(BytesStreamFixture, Test1) {
  // Arrange
  constexpr std::string_view expected = "Hello!5123.456";
  stream_.exceptions(std::ifstream::failbit | std::ifstream::badbit | std::ifstream::eofbit);

  // Act
  stream_ << "Hello";
  stream_ << '!';
  stream_ << 5;
  stream_ << 123.456;

  // Assert
  EXPECT_EQ(expected, std::string_view(slice_.data(), slice_.size()));
  EXPECT_EQ(expected.size(), stream_.tellp());
}

// This fixture changes only the length for tellp(). It never accesses element storage.
class PositionSlice : public Slice<char> {
 public:
  explicit PositionSlice(size_t size) noexcept { set_size(size); }
  ~PositionSlice() { set_size(0); }
};

static_assert(std::cmp_greater(std::numeric_limits<size_t>::max(), std::numeric_limits<std::streamoff>::max()));

TEST(BytesStreamPositionTest, TellpAtMaximumStreamOffset) {
  // Arrange
  constexpr auto kMaximumOffset = std::numeric_limits<std::streamoff>::max();
  PositionSlice slice(static_cast<size_t>(kMaximumOffset));
  BytesStream stream(&slice);

  // Act
  const auto position = stream.tellp();

  // Assert
  EXPECT_EQ(std::streampos(kMaximumOffset), position);
}

TEST(BytesStreamPositionTest, TellpRejectsUnrepresentableSize) {
  // Arrange
  const auto size = static_cast<size_t>(std::numeric_limits<std::streamoff>::max()) + 1;
  PositionSlice slice(size);
  BytesStream stream(&slice);

  // Act
  const auto position = stream.tellp();

  // Assert
  EXPECT_EQ(std::streampos(static_cast<std::streamoff>(-1)), position);
}

}  // namespace
