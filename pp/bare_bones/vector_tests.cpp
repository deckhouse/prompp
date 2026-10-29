#include <gtest/gtest.h>

#include <random>
#include <sstream>

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

class BareBonesVectorFixture : public testing::Test {};

TEST_F(BareBonesVectorFixture, CopyAssignmentReleasesReplacedNestedElements) {
  // Arrange
  Vector<Vector<uint32_t>> source{{1U, 2U}};
  Vector<Vector<uint32_t>> destination{{3U}, {4U, 5U}};

  // Act
  destination = source;
  source[0][0] = 9U;

  // Assert
  ASSERT_EQ(1U, destination.size());
  ASSERT_EQ(2U, destination[0].size());
  EXPECT_EQ(1U, destination[0][0]);
  EXPECT_EQ(2U, destination[0][1]);
}

TEST_F(BareBonesVectorFixture, EmptyCopyAssignmentReleasesNestedElements) {
  // Arrange
  Vector<Vector<uint32_t>> source;
  Vector<Vector<uint32_t>> destination{{3U}, {4U, 5U}};

  // Act
  destination = source;

  // Assert
  EXPECT_TRUE(destination.empty());
}

TEST_F(BareBonesVectorFixture, GrowingCopyAssignmentPreservesNestedValues) {
  // Arrange
  Vector<Vector<uint32_t>> source{{1U}, {2U, 3U}, {4U}};
  Vector<Vector<uint32_t>> destination{{5U, 6U}};

  // Act
  destination = source;

  // Assert
  ASSERT_EQ(3U, destination.size());
  EXPECT_EQ(source, destination);
}

TEST_F(BareBonesVectorFixture, InitializerListConstructor) {
  // Arrange

  // Act
  Vector<std::string_view> vector{"123", "456", "789"};

  // Assert
  EXPECT_EQ(3U, vector.size());
  EXPECT_EQ("123", vector[0]);
  EXPECT_EQ("456", vector[1]);
  EXPECT_EQ("789", vector[2]);
}

TEST_F(BareBonesVectorFixture, PushBack) {
  // Arrange
  Vector<uint32_t> vector;
  std::array items{1U, 2U, 3U, 4U, 5U};

  // Act
  vector.push_back(items.begin(), items.end());

  // Assert
  EXPECT_TRUE(std::ranges::equal(vector, items));
}

TEST_F(BareBonesVectorFixture, PushBackNonTriviallyCopyableObjects) {
  // Arrange
  Vector<Vector<uint32_t>> vector;
  std::array items{Vector{1U, 2U, 3U}, Vector{4U, 5U, 6U}};

  // Act
  vector.push_back(items.begin(), items.end());

  // Assert
  EXPECT_TRUE(std::ranges::equal(vector, items));
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

struct VectorValuesCase {
  const char* name;
  std::vector<uint32_t> values;
};

class BareBonesVectorValuesFixture : public testing::TestWithParam<VectorValuesCase> {
 protected:
  [[nodiscard]] static std::vector<uint32_t> mixed_values() {
    std::mt19937 generator(42);
    std::vector<uint32_t> values(100);
    std::ranges::generate(values, [&generator] { return generator(); });
    return values;
  }

 public:
  static auto cases() {
    return testing::Values(VectorValuesCase{.name = "Empty", .values = {}}, VectorValuesCase{.name = "Zeros", .values = std::vector<uint32_t>(100, 0)},
                           VectorValuesCase{.name = "MixedValues", .values = mixed_values()});
  }
};

TEST_P(BareBonesVectorValuesFixture, PreservesInsertionOrder) {
  // Arrange
  Vector<uint32_t> actual;

  // Act
  std::ranges::copy(GetParam().values, std::back_inserter(actual));

  // Assert
  EXPECT_TRUE(std::ranges::equal(GetParam().values, actual));
}

TEST_P(BareBonesVectorValuesFixture, StreamRoundTripPreservesValuesAndSaveSize) {
  // Arrange
  Vector<uint32_t> source;
  std::ranges::copy(GetParam().values, std::back_inserter(source));
  Vector<uint32_t> restored;
  std::stringstream stream;

  // Act
  stream << source;
  stream >> restored;

  // Assert
  EXPECT_EQ(source.save_size(), stream.str().size());
  EXPECT_TRUE(std::ranges::equal(GetParam().values, restored));
}

INSTANTIATE_TEST_SUITE_P(EmptyZerosAndMixedValues,
                         BareBonesVectorValuesFixture,
                         BareBonesVectorValuesFixture::cases(),
                         [](const testing::TestParamInfo<VectorValuesCase>& info) { return info.param.name; });

}  // namespace
