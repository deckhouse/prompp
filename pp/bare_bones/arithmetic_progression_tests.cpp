#include <limits>

#include <gmock/gmock.h>

#include "bare_bones/arithmetic_progression.h"

namespace {
using BareBones::ArithmeticProgression;
static_assert(std::ranges::random_access_range<ArithmeticProgression>);
static_assert(std::ranges::borrowed_range<ArithmeticProgression>);
static_assert(!std::ranges::contiguous_range<ArithmeticProgression>);
TEST(ArithmeticProgressionTest, DefaultProgressionIsEmpty) {
  // Arrange

  // Act
  const ArithmeticProgression values;

  // Assert
  EXPECT_EQ(0U, values.size());
  EXPECT_THAT(values, testing::ElementsAre());
}

TEST(ArithmeticProgressionTest, ValuesStartAtFirstAndAdvanceByStep) {
  // Arrange

  // Act
  const ArithmeticProgression values(2, 2, 3);

  // Assert
  EXPECT_EQ(2U, values.first());
  EXPECT_EQ(2, values.step());
  EXPECT_EQ(3U, values.size());
  EXPECT_THAT(values, testing::ElementsAre(2, 4, 6));
}

TEST(ArithmeticProgressionTest, ZeroStepRepeatsOneValue) {
  // Arrange

  // Act
  const ArithmeticProgression values(7, 0, 3);

  // Assert
  EXPECT_THAT(values, testing::ElementsAre(7, 7, 7));
}

TEST(ArithmeticProgressionTest, NegativeStepDecreasesValues) {
  // Arrange

  // Act
  const ArithmeticProgression values(6, -2, 3);

  // Assert
  EXPECT_THAT(values, testing::ElementsAre(6, 4, 2));
}

TEST(ArithmeticProgressionTest, IteratorSupportsRandomAccess) {
  // Arrange
  const ArithmeticProgression values(10, -2, 4);

  // Act
  const auto middle = values.begin() + 2;

  // Assert
  EXPECT_EQ(6U, *middle);
  EXPECT_EQ(4, values.end() - values.begin());
  EXPECT_THAT(values, testing::ElementsAre(10, 8, 6, 4));
}

TEST(ArithmeticProgressionTest, IteratorSurvivesTemporaryRange) {
  // Arrange
  const auto it = ArithmeticProgression(7, 3, 2).begin();

  // Act
  const auto next = it + 1;

  // Assert
  EXPECT_EQ(10U, *next);
}

TEST(ArithmeticProgressionTest, EmptyEqualityIgnoresFirstAndStep) {
  // Arrange
  const ArithmeticProgression empty(99, std::numeric_limits<int64_t>::min(), 0);

  // Act
  const auto equal = empty == ArithmeticProgression{};

  // Assert
  EXPECT_TRUE(equal);
}

TEST(ArithmeticProgressionTest, SingletonEqualityIgnoresStep) {
  // Arrange
  const ArithmeticProgression singleton(7, std::numeric_limits<int64_t>::max(), 1);

  // Act
  const auto equal = singleton == ArithmeticProgression(7, 0, 1);

  // Assert
  EXPECT_TRUE(equal);
}

TEST(ArithmeticProgressionTest, MaximumSizeHasRepresentableEndDistance) {
  // Arrange
  const ArithmeticProgression values(0, 1, std::numeric_limits<uint32_t>::max());

  // Act
  const auto distance = values.end() - values.begin();

  // Assert
  EXPECT_EQ(4294967295LL, distance);
  EXPECT_EQ(-4294967295LL, values.begin() - values.end());
  EXPECT_EQ(4294967294U, values.back());
}

TEST(ArithmeticProgressionTest, EndDoesNotComputeAnOutOfRangeValue) {
  // Arrange
  const ArithmeticProgression values(4294967294U, 1, 2);

  // Act
  const auto distance = values.end() - values.begin();

  // Assert
  EXPECT_EQ(2, distance);
  EXPECT_EQ(4294967295U, values.back());
}

TEST(ArithmeticProgressionTest, DecrementFromWrappedEndRestoresLastValue) {
  // Arrange
  const ArithmeticProgression values(4294967294U, 1, 2);
  auto it = values.end();

  // Act
  --it;

  // Assert
  EXPECT_EQ(4294967295U, *it);
  EXPECT_EQ(values.begin() + 1, it);
}

TEST(ArithmeticProgressionTest, NegativeStepAdvancesAcrossSignedBoundary) {
  // Arrange
  const ArithmeticProgression values(4294967295U, -2147483648LL, 2);
  auto it = values.begin();

  // Act
  ++it;

  // Assert
  EXPECT_EQ(2147483647U, *it);
  EXPECT_EQ(values.begin() + 1, it);
}

TEST(ArithmeticProgressionTest, RandomNavigationRestoresTheSamePosition) {
  // Arrange
  const ArithmeticProgression values(10, -2, 4);
  auto it = values.end();

  // Act
  it -= 2;
  it += -1;

  // Assert
  EXPECT_EQ(8U, *it);
  EXPECT_EQ(values.begin() + 1, it);
  EXPECT_EQ(1, it - values.begin());
}

TEST(ArithmeticProgressionTest, EqualValuesAtDifferentPositionsRemainDistinctIterators) {
  // Arrange
  const ArithmeticProgression values(7, 0, 3);
  const auto first = values.begin();
  const auto second = first + 1;

  // Act
  const auto equal = first == second;

  // Assert
  EXPECT_EQ(7U, *first);
  EXPECT_EQ(7U, *second);
  EXPECT_FALSE(equal);
  EXPECT_LT(first, second);
}

TEST(ArithmeticProgressionTest, OrderingFollowsPositionForNegativeStep) {
  // Arrange
  const ArithmeticProgression values(10, -2, 3);
  const auto first = values.begin() + 1;
  const auto second = values.begin() + 2;

  // Act
  const auto less = first < second;

  // Assert
  EXPECT_EQ(8U, *first);
  EXPECT_EQ(6U, *second);
  EXPECT_TRUE(less);
}

}  // namespace
