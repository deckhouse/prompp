#include <gtest/gtest.h>

#include <limits>
#include <random>
#include <sstream>

#include "bare_bones/vector.h"

namespace {

using BareBones::Vector;

class VectorFailureFixture : public testing::Test {
 protected:
  struct ConstructionFailure {};

  struct State {
    int live{};
    int copies_left{std::numeric_limits<int>::max()};
    int defaults_left{std::numeric_limits<int>::max()};
    bool fail_allocation{};
  };

  // Default construction and the allocator API need a static binding to fixture-owned state.
  static inline State* active_state_{};

  struct Reallocator : BareBones::DefaultReallocator {
    static void* reallocate(void* memory, size_t size) { return active_state_->fail_allocation ? nullptr : DefaultReallocator::reallocate(memory, size); }
  };

  struct Value {
    int value = 42;

    Value() {
      if (active_state_->defaults_left == 0) {
        throw ConstructionFailure{};
      }
      --active_state_->defaults_left;
      ++active_state_->live;
    }
    Value(const Value& other) : value(other.value) {
      if (active_state_->copies_left == 0) {
        throw ConstructionFailure{};
      }
      --active_state_->copies_left;
      ++active_state_->live;
    }
    Value(Value&& other) noexcept : value(other.value) { ++active_state_->live; }
    Value& operator=(const Value&) = delete;
    Value& operator=(Value&&) = delete;
    ~Value() { --active_state_->live; }
  };

  State state_;

  void SetUp() override { active_state_ = &state_; }
  void TearDown() override {
    active_state_ = nullptr;
    EXPECT_EQ(0, state_.live);
  }
};

TEST_F(VectorFailureFixture, FailedObjectGrowthPreservesLiveElements) {
  // Arrange
  Vector<Value, Reallocator> values;
  values.emplace_back();
  const auto* allocation = values.data();
  const auto capacity = values.capacity();
  state_.fail_allocation = true;

  // Act
  EXPECT_THROW(values.reserve(capacity + 1), std::bad_alloc);

  // Assert
  EXPECT_EQ(allocation, values.data());
  EXPECT_EQ(capacity, values.capacity());
  ASSERT_EQ(1U, values.size());
  EXPECT_EQ(42, values[0].value);
  EXPECT_EQ(1, state_.live);
}

TEST_F(VectorFailureFixture, FailedDefaultConstructionDestroysPartialTail) {
  // Arrange
  Vector<Value> values;
  values.emplace_back();
  state_.defaults_left = 1;

  // Act
  EXPECT_THROW(values.resize(4), ConstructionFailure);

  // Assert
  ASSERT_EQ(1U, values.size());
  EXPECT_EQ(42, values[0].value);
  EXPECT_EQ(1, state_.live);
}

TEST_F(VectorFailureFixture, FailedFillConstructionDestroysPartialTail) {
  // Arrange
  Vector<Value> values;
  values.emplace_back();
  Value value;
  state_.copies_left = 1;

  // Act
  EXPECT_THROW(values.resize(4, value), ConstructionFailure);

  // Assert
  EXPECT_EQ(1U, values.size());
  EXPECT_EQ(2, state_.live);
}

TEST_F(VectorFailureFixture, FailedCopyConstructorDestroysPartialCopy) {
  // Arrange
  Vector<Value> source;
  source.resize(3);
  state_.copies_left = 1;
  const auto copy = [&] { return source; };

  // Act
  EXPECT_THROW(static_cast<void>(copy()), ConstructionFailure);

  // Assert
  EXPECT_EQ(3U, source.size());
  EXPECT_EQ(3, state_.live);
}

TEST_F(VectorFailureFixture, FailedCopyAssignmentLeavesValidEmptyDestination) {
  // Arrange
  Vector<Value> source;
  source.resize(3);
  Vector<Value> destination;
  destination.resize(2);
  state_.copies_left = 1;

  // Act
  EXPECT_THROW(destination = source, ConstructionFailure);

  // Assert
  EXPECT_TRUE(destination.empty());
  EXPECT_EQ(3U, source.size());
  EXPECT_EQ(3, state_.live);
}

TEST_F(VectorFailureFixture, FailedInsertCopyPreservesValues) {
  // Arrange
  Vector<Value> values;
  values.resize(2);
  values[1].value = 84;
  Value value;
  state_.copies_left = 0;

  // Act
  EXPECT_THROW(values.insert(values.begin(), value), ConstructionFailure);

  // Assert
  ASSERT_EQ(2U, values.size());
  EXPECT_EQ(3, state_.live);
  EXPECT_EQ(42, values[0].value);
  EXPECT_EQ(84, values[1].value);
}

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

TEST_F(BareBonesVectorFixture, PairInsertionsPreserveBorrowedValues) {
  // Arrange
  using Pair = std::pair<std::string_view, std::string_view>;
  Vector<Pair> values{{"a", "1"}, {"c", "3"}};
  const std::array<Pair, 5> expected{{{"0", "0"}, {"a", "1"}, {"b", "2"}, {"c", "3"}, {"d", "4"}}};

  // Act
  values.insert(values.begin() + 1, Pair{"b", "2"});
  values.insert(values.begin(), Pair{"0", "0"});
  values.insert(values.end(), Pair{"d", "4"});

  // Assert
  EXPECT_TRUE(std::ranges::equal(expected, values));
}

TEST_F(BareBonesVectorFixture, ResizeConstructsEmptyUniqueOwners) {
  // Arrange
  Vector<std::unique_ptr<uint32_t>> values;

  // Act
  values.resize(3);
  values.reserve(values.capacity() + 1);
  values.shrink_to_fit();

  // Assert
  ASSERT_EQ(3U, values.size());
  EXPECT_EQ(nullptr, values[0]);
  EXPECT_EQ(nullptr, values[1]);
  EXPECT_EQ(nullptr, values[2]);
}

TEST_F(BareBonesVectorFixture, GrowthPreservesStandardVectorElements) {
  // Arrange
  Vector<std::vector<uint32_t>> values;
  values.emplace_back(std::initializer_list<uint32_t>{1U, 2U});
  const auto first_data = values[0].data();

  // Act
  values.resize(3);
  values.reserve(values.capacity() + 1);
  values.shrink_to_fit();

  // Assert
  ASSERT_EQ(3U, values.size());
  EXPECT_EQ(first_data, values[0].data());
  EXPECT_EQ((std::vector<uint32_t>{1U, 2U}), values[0]);
  EXPECT_TRUE(values[1].empty());
  EXPECT_TRUE(values[2].empty());
}

class VectorOwnershipFixture : public testing::Test {
 protected:
  struct Value {
    int* destructions;
    ~Value() { ++*destructions; }
  };
};

TEST_F(VectorOwnershipFixture, GrowthAndEraseReleaseEachOwnedValueOnce) {
  // Arrange
  int destructions = 0;
  Vector<std::unique_ptr<Value>> values;
  values.emplace_back(std::make_unique<Value>(&destructions));
  values.emplace_back(std::make_unique<Value>(&destructions));
  values.emplace_back(std::make_unique<Value>(&destructions));
  const auto surviving_value = values[1].get();

  // Act
  values.reserve(values.capacity() + 1);
  values.erase(values.begin());

  // Assert
  EXPECT_EQ(1, destructions);
  ASSERT_EQ(2U, values.size());
  EXPECT_EQ(surviving_value, values[0].get());
  values.clear();
  EXPECT_EQ(3, destructions);
}

TEST_F(VectorOwnershipFixture, MoveAssignmentDestroysReplacedOwners) {
  // Arrange
  int destructions = 0;
  Vector<std::unique_ptr<Value>> source;
  source.emplace_back(std::make_unique<Value>(&destructions));
  Vector<std::unique_ptr<Value>> destination;
  destination.emplace_back(std::make_unique<Value>(&destructions));
  const auto* surviving_value = source[0].get();

  // Act
  destination = std::move(source);

  // Assert
  EXPECT_EQ(1, destructions);
  // Move assignment guarantees an empty source that remains usable.
  // NOLINTNEXTLINE(bugprone-use-after-move)
  EXPECT_TRUE(source.empty());
  ASSERT_EQ(1U, destination.size());
  EXPECT_EQ(surviving_value, destination[0].get());
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
