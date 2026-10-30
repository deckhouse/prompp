#include <array>
#include <ranges>
#include <vector>

#include <gtest/gtest.h>

#include "bare_bones/vector_with_holes.h"

namespace {

using BareBones::VectorWithHoles;

template <class T>
class ValueOnHeap {
 public:
  explicit ValueOnHeap(T value) : value_(new T(value)) {}
  ~ValueOnHeap() {
    delete value_;
    value_ = nullptr;
  }

  PROMPP_ALWAYS_INLINE bool operator==(const uint32_t value) const noexcept { return value == *value_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return sizeof(T); }

 private:
  const T* value_;
};

template <class T>
class VectorWithHolesFixture : public testing::Test {};

typedef testing::Types<uint32_t, ValueOnHeap<uint32_t>> VectorWithHolesFixtureTypes;
TYPED_TEST_SUITE(VectorWithHolesFixture, VectorWithHolesFixtureTypes);

TYPED_TEST(VectorWithHolesFixture, EmplaceAtAllocatedMemory) {
  // Arrange
  VectorWithHoles<TypeParam> vector;

  // Act
  vector.emplace_back(101U);

  // Assert
  EXPECT_EQ(1U, vector.size());
  EXPECT_EQ(101U, vector[0]);
}

TYPED_TEST(VectorWithHolesFixture, EmplaceAtHole) {
  // Arrange
  VectorWithHoles<TypeParam> vector;
  vector.emplace_back(101U);

  // Act
  vector.erase(0);
  vector.emplace_back(102U);

  // Assert
  EXPECT_EQ(1U, vector.size());
  EXPECT_EQ(102U, vector[0]);
}

TYPED_TEST(VectorWithHolesFixture, EmplaceAfterEmplaceAtHole) {
  // Arrange
  VectorWithHoles<TypeParam> vector;
  vector.emplace_back(101U);

  // Act
  vector.erase(0);
  vector.emplace_back(102U);
  vector.emplace_back(103U);

  // Assert
  EXPECT_EQ(2U, vector.size());
  EXPECT_EQ(102U, vector[0]);
  EXPECT_EQ(103U, vector[1]);
}

TYPED_TEST(VectorWithHolesFixture, MultiplyHoles) {
  // Arrange
  VectorWithHoles<TypeParam> vector;
  vector.emplace_back(101U);
  vector.emplace_back(102U);
  vector.emplace_back(103U);

  // Act
  vector.erase(0);
  vector.erase(1);
  vector.emplace_back(104U);
  vector.emplace_back(105U);

  // Assert
  EXPECT_EQ(3U, vector.size());
  EXPECT_EQ(105U, vector[0]);
  EXPECT_EQ(104U, vector[1]);
  EXPECT_EQ(103U, vector[2]);
}

TYPED_TEST(VectorWithHolesFixture, DestructorWithHoles) {
  // Arrange
  VectorWithHoles<TypeParam> vector;
  vector.emplace_back(101U);
  vector.emplace_back(102U);
  vector.emplace_back(103U);

  // Act
  vector.erase(0);
  vector.erase(2);

  // Assert
  EXPECT_EQ(3U, vector.size());
}

TYPED_TEST(VectorWithHolesFixture, CheckAllocatedEmpty) {
  // Arrange
  VectorWithHoles<TypeParam> vector;

  // Act

  // Assert
  EXPECT_EQ(0U, vector.allocated_memory());
}

TYPED_TEST(VectorWithHolesFixture, CheckAllocatedDiff) {
  // Arrange
  VectorWithHoles<TypeParam> vector;

  // Act
  vector.emplace_back(101U);
  vector.emplace_back(102U);
  vector.emplace_back(103U);

  vector.erase(0);
  vector.erase(1);
  vector.erase(2);

  vector.emplace_back(101U);
  vector.emplace_back(102U);
  vector.emplace_back(103U);

  const size_t allocated_memory_full = vector.allocated_memory();
  const size_t mid_allocated_memory = BareBones::mem::allocated_memory(vector[1]);

  vector.erase(1);
  const size_t allocated_memory_erased = vector.allocated_memory();

  // Assert
  EXPECT_EQ(mid_allocated_memory, allocated_memory_full - allocated_memory_erased);
}

TYPED_TEST(VectorWithHolesFixture, CheckValueAccess) {
  // Arrange
  VectorWithHoles<TypeParam> vector;
  vector.emplace_back(101U);
  vector.emplace_back(102U);
  vector.emplace_back(103U);

  // Act
  const auto& value_bracket = vector[0];
  const auto& value_at = vector.at(0);

  // Assert
  EXPECT_EQ(value_bracket, 101U);
  EXPECT_EQ(value_at, 101U);
}

TYPED_TEST(VectorWithHolesFixture, CheckHoleAccessFail) {
  // Arrange
  VectorWithHoles<TypeParam> vector;
  vector.emplace_back(101U);
  vector.emplace_back(102U);
  vector.emplace_back(103U);

  // Act
  vector.erase(0);

  // Assert
  ASSERT_THROW({ vector.at(0); }, BareBones::Exception);
}

TEST(VectorWithHolesTest, IteratorSkipsHoles) {
  // Arrange
  VectorWithHoles<ValueOnHeap<uint32_t>> vector;
  vector.emplace_back(101U);
  vector.emplace_back(102U);
  vector.emplace_back(103U);
  vector.emplace_back(104U);
  vector.erase(0);
  vector.erase(2);
  const auto& const_vector = vector;

  // Act
  const auto visited = const_vector | std::views::transform([](const ValueOnHeap<uint32_t>& value) { return &value; }) | std::ranges::to<std::vector>();

  // Assert
  EXPECT_EQ((std::vector{&const_vector[1], &const_vector[3]}), visited);
}

TEST(VectorWithHolesTest, RangeDistanceSkipsHoles) {
  // Arrange
  VectorWithHoles<ValueOnHeap<uint32_t>> vector;
  vector.emplace_back(101U);
  vector.emplace_back(102U);
  vector.emplace_back(103U);
  vector.erase(1);
  const auto& const_vector = vector;

  // Act
  const auto distance = std::ranges::distance(const_vector);

  // Assert
  EXPECT_EQ(2, distance);
}

TEST(VectorWithHolesTest, IteratorOfFullyErasedVectorIsEmpty) {
  // Arrange
  VectorWithHoles<ValueOnHeap<uint32_t>> vector;
  vector.emplace_back(101U);
  vector.erase(0);
  const auto& const_vector = vector;

  // Act
  const auto is_empty = const_vector.begin() == const_vector.end();

  // Assert
  EXPECT_TRUE(is_empty);
}

struct DestroyCounts {
  int with_argument{};
  int without_argument{};
  int last_argument{};
  int destructors{};
};

class ObservedDestroy {
 public:
  explicit ObservedDestroy(DestroyCounts* counts) noexcept : counts_(counts) {}
  ~ObservedDestroy() noexcept { ++counts_->destructors; }

  void destroy(int argument) noexcept {
    ++counts_->with_argument;
    counts_->last_argument = argument;
  }
  void destroy() noexcept { ++counts_->without_argument; }

 private:
  DestroyCounts* counts_;
};

TEST(VectorWithHolesTest, EraseCallsCorrectDestroy) {
  // Arrange
  std::array<DestroyCounts, 3> counts{};

  // Act
  {
    VectorWithHoles<ObservedDestroy> vector;
    vector.emplace_back(&counts[0]);
    vector.emplace_back(&counts[1]);
    vector.emplace_back(&counts[2]);
    vector.erase(0, 42);
    vector.erase(1);
  }

  // Assert
  EXPECT_EQ(1, counts[0].with_argument);
  EXPECT_EQ(42, counts[0].last_argument);
  EXPECT_EQ(0, counts[0].without_argument);
  EXPECT_EQ(1, counts[0].destructors);
  EXPECT_EQ(0, counts[1].with_argument);
  EXPECT_EQ(1, counts[1].without_argument);
  EXPECT_EQ(1, counts[1].destructors);
  EXPECT_EQ(0, counts[2].with_argument);
  EXPECT_EQ(1, counts[2].without_argument);
  EXPECT_EQ(1, counts[2].destructors);
}

}  // namespace