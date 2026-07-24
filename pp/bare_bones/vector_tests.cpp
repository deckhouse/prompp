#include <gtest/gtest.h>

#include "bare_bones/vector.h"

namespace {

using BareBones::Vector;

class BareBonesVectorAllocatedMemoryFixture : public testing::Test {
 protected:
  struct Object {
    [[nodiscard]] static size_t allocated_memory() noexcept { return kObjectAllocatedMemory; }
  };

  static constexpr size_t kObjectAllocatedMemory = 1;
};

TEST_F(BareBonesVectorAllocatedMemoryFixture, ObjectWithoutAllocatedMemoryMethod) {
  // Arrange
  using Vector = Vector<uint8_t>;

  Vector vector;
  vector.emplace_back(1);
  vector.emplace_back(2);
  vector.emplace_back(3);

  // Act
  const auto allocated_memory = vector.allocated_memory();

  // Assert
  EXPECT_EQ(vector.capacity() * sizeof(Vector::value_type), allocated_memory);
}

TEST_F(BareBonesVectorAllocatedMemoryFixture, ObjectWithAllocatedMemoryMethod) {
  // Arrange
  using Vector = Vector<Object>;

  Vector vector;
  vector.emplace_back();
  vector.emplace_back();
  vector.emplace_back();

  // Act
  const auto allocated_memory = vector.allocated_memory();

  // Assert
  EXPECT_EQ(vector.capacity() * sizeof(Vector::value_type) + vector.size() * kObjectAllocatedMemory, allocated_memory);
}

TEST_F(BareBonesVectorAllocatedMemoryFixture, PointerWithAllocatedMemoryMethod) {
  // Arrange
  using Vector = Vector<Object*>;

  Object object;

  Vector vector;
  vector.emplace_back(&object);
  vector.emplace_back(&object);
  vector.emplace_back(&object);

  // Act
  const auto allocated_memory = vector.allocated_memory();

  // Assert
  EXPECT_EQ(
      vector.capacity() * sizeof(Vector::value_type)  // NOLINT(bugprone-sizeof-expression): value_type is intentionally a pointer; we want the pointer size.
          + vector.size() * kObjectAllocatedMemory,
      allocated_memory);
}

TEST_F(BareBonesVectorAllocatedMemoryFixture, DereferencableWithAllocatedMemoryMethod) {
  // Arrange
  using Vector = Vector<std::unique_ptr<Object>>;

  Vector vector;
  vector.emplace_back(std::make_unique<Object>());
  vector.emplace_back(std::make_unique<Object>());
  vector.emplace_back(std::make_unique<Object>());

  // Act
  const auto allocated_memory = vector.allocated_memory();

  // Assert
  EXPECT_EQ(vector.capacity() * sizeof(Vector::value_type) + vector.size() * kObjectAllocatedMemory, allocated_memory);
}

TEST(BareBonesVector, InitializerListConstructor) {
  // Arrange

  // Act
  Vector<std::string_view> vector{"123", "456", "789"};

  // Assert
  EXPECT_EQ(3U, vector.size());
  EXPECT_EQ("123", vector[0]);
  EXPECT_EQ("456", vector[1]);
  EXPECT_EQ("789", vector[2]);
}

class BareBonesVectorEraseFixture : public testing::Test {
 protected:
  Vector<std::unique_ptr<std::string_view>> vector_;

  void SetUp() override {
    vector_.emplace_back(std::make_unique<std::string_view>("1"));
    vector_.emplace_back(std::make_unique<std::string_view>("2"));
    vector_.emplace_back(std::make_unique<std::string_view>("3"));
  }
};

TEST_F(BareBonesVectorEraseFixture, EraseLastItemByRange) {
  // Arrange

  // Act
  vector_.erase(vector_.end() - 1, vector_.end());

  // Assert
  EXPECT_EQ(2U, vector_.size());
  EXPECT_EQ("1", *vector_[0]);
  EXPECT_EQ("2", *vector_[1]);
}

TEST_F(BareBonesVectorEraseFixture, EraseLastItem) {
  // Arrange

  // Act
  vector_.erase(vector_.end() - 1);

  // Assert
  EXPECT_EQ(2U, vector_.size());
  EXPECT_EQ("1", *vector_[0]);
  EXPECT_EQ("2", *vector_[1]);
}

TEST_F(BareBonesVectorEraseFixture, EraseFirstItemByRange) {
  // Arrange

  // Act
  vector_.erase(vector_.begin(), vector_.begin() + 1);

  // Assert
  EXPECT_EQ(2U, vector_.size());
  EXPECT_EQ("2", *vector_[0]);
  EXPECT_EQ("3", *vector_[1]);
}

TEST_F(BareBonesVectorEraseFixture, EraseFirstItem) {
  // Arrange

  // Act
  vector_.erase(vector_.begin());

  // Assert
  EXPECT_EQ(2U, vector_.size());
  EXPECT_EQ("2", *vector_[0]);
  EXPECT_EQ("3", *vector_[1]);
}

TEST_F(BareBonesVectorEraseFixture, EraseSecondItemByRange) {
  // Arrange

  // Act
  vector_.erase(vector_.begin() + 1, vector_.begin() + 2);

  // Assert
  EXPECT_EQ(2U, vector_.size());
  EXPECT_EQ("1", *vector_[0]);
  EXPECT_EQ("3", *vector_[1]);
}

TEST_F(BareBonesVectorEraseFixture, EraseSecondItem) {
  // Arrange

  // Act
  vector_.erase(vector_.begin() + 1);

  // Assert
  EXPECT_EQ(2U, vector_.size());
  EXPECT_EQ("1", *vector_[0]);
  EXPECT_EQ("3", *vector_[1]);
}

TEST_F(BareBonesVectorEraseFixture, EraseAllItems) {
  // Arrange

  // Act
  vector_.erase(vector_.begin(), vector_.end());

  // Assert
  EXPECT_TRUE(vector_.empty());
}

}  // namespace
