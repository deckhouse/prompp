#include <algorithm>
#include <array>
#include <atomic>
#include <barrier>
#include <cstdint>
#include <cstring>
#include <numeric>
#include <thread>

#include <gtest/gtest.h>

#include "bare_bones/memory.h"

namespace {

using BareBones::DefaultReallocator;
using BareBones::Memory;
using BareBones::MemoryControlBlock;
using BareBones::MemoryControlBlockWithItemCount;
using BareBones::PreAllocationSizeCalculator;
using BareBones::SharedMemory;
using BareBones::SharedPtr;
using BareBones::SharedPtrControlBlockWithItemCount;

struct GetAllocationSizeCase {
  uint32_t needed_size;
  uint32_t expected_size;
};

class AllocationSizeCalculatorFixture : public testing::TestWithParam<GetAllocationSizeCase> {
 protected:
  using Calculator = PreAllocationSizeCalculator<char, uint32_t>;
};

TEST_P(AllocationSizeCalculatorFixture, TestCalculate) {
  // Arrange

  // Act
  const auto size = Calculator::calculate(GetParam().needed_size);

  // Assert
  EXPECT_EQ(GetParam().expected_size, size);
}

INSTANTIATE_TEST_SUITE_P(
    UpTo50PercentAlignedToMinAllocationSize,
    AllocationSizeCalculatorFixture,
    testing::Values(GetAllocationSizeCase{.needed_size = 0, .expected_size = PreAllocationSizeCalculator<char, uint32_t>::kMinAllocationSize},
                    GetAllocationSizeCase{.needed_size = static_cast<uint32_t>(PreAllocationSizeCalculator<char, uint32_t>::kMinAllocationSize * 0.66),
                                          .expected_size = PreAllocationSizeCalculator<char, uint32_t>::kMinAllocationSize},
                    GetAllocationSizeCase{.needed_size = static_cast<uint32_t>(PreAllocationSizeCalculator<char, uint32_t>::kMinAllocationSize * 0.66) + 1,
                                          .expected_size = PreAllocationSizeCalculator<char, uint32_t>::kMinAllocationSize * 2},
                    GetAllocationSizeCase{.needed_size = 170, .expected_size = 256},
                    GetAllocationSizeCase{.needed_size = 171, .expected_size = 512}));

INSTANTIATE_TEST_SUITE_P(UpTo50PercentAlignedTo256,
                         AllocationSizeCalculatorFixture,
                         testing::Values(GetAllocationSizeCase{.needed_size = 171, .expected_size = 512},
                                         GetAllocationSizeCase{.needed_size = 2730, .expected_size = 4096},
                                         GetAllocationSizeCase{.needed_size = 2731, .expected_size = 4096},
                                         GetAllocationSizeCase{.needed_size = 2732, .expected_size = 4096}));

INSTANTIATE_TEST_SUITE_P(UpTo10PercentAlignedTo4096,
                         AllocationSizeCalculatorFixture,
                         testing::Values(GetAllocationSizeCase{.needed_size = 4096, .expected_size = 8192}));

class MemoryFixture : public ::testing::Test {
 protected:
  Memory<MemoryControlBlock, uint8_t> memory_;
};

TEST_F(MemoryFixture, Empty) {
  // Arrange

  // Act

  // Assert
  EXPECT_EQ(0U, memory_.size());
  EXPECT_EQ(nullptr, memory_);
}

TEST_F(MemoryFixture, CopyConstructor) {
  // Arrange
  memory_.resize_to_fit_at_least(1);
  std::iota(memory_.begin(), memory_.end(), uint8_t{});

  // Act
  const auto memory2 = memory_;

  // Assert
  EXPECT_NE(memory2, memory_);
  ASSERT_EQ(memory_.size(), memory2.size());
  EXPECT_TRUE(std::ranges::equal(memory2, memory_));
}

TEST_F(MemoryFixture, CopyOperator) {
  // Arrange
  memory_.resize_to_fit_at_least(1);
  std::iota(memory_.begin(), memory_.end(), uint8_t{});

  // Act
  Memory<MemoryControlBlock, uint8_t> memory2;
  memory2.resize_to_fit_at_least(1);
  memory2 = memory_;

  // Assert
  EXPECT_NE(memory2, memory_);
  ASSERT_EQ(memory_.size(), memory2.size());
  EXPECT_TRUE(std::ranges::equal(memory2, memory_));
}

TEST_F(MemoryFixture, MoveConstructor) {
  // Arrange
  memory_.resize_to_fit_at_least(1);
  const auto memory_ptr = memory_.begin();
  const auto memory_size = memory_.size();

  // Act
  const auto memory2 = std::move(memory_);

  // Assert
  EXPECT_EQ(0U, memory_.size());
  EXPECT_EQ(nullptr, memory_);

  EXPECT_EQ(memory_ptr, memory2);
  EXPECT_EQ(memory_size, memory2.size());
}

TEST_F(MemoryFixture, MoveOperator) {
  // Arrange
  memory_.resize_to_fit_at_least(1);
  const auto memory_ptr = memory_.begin();
  const auto memory_size = memory_.size();

  // Act
  Memory<MemoryControlBlock, uint8_t> memory2;
  memory2.resize_to_fit_at_least(1);
  memory2 = std::move(memory_);

  // Assert
  EXPECT_EQ(0U, memory_.size());
  EXPECT_EQ(nullptr, memory_);

  EXPECT_EQ(memory_ptr, memory2);
  EXPECT_EQ(memory_size, memory2.size());
}

class MemoryWithItemCountFixture : public ::testing::Test {
 protected:
  Memory<MemoryControlBlockWithItemCount, uint8_t> memory_;
};

TEST_F(MemoryWithItemCountFixture, CopyConstructor) {
  // Arrange
  memory_.resize_to_fit_at_least(1);
  std::iota(memory_.begin(), memory_.end(), uint8_t{});
  memory_.control_block().items_count = 100;

  // Act
  const auto memory2 = memory_;

  // Assert
  EXPECT_NE(memory2, memory_);
  ASSERT_EQ(memory_.size(), memory2.size());
  ASSERT_EQ(memory_.control_block().data_size, memory2.control_block().data_size);
  ASSERT_EQ(memory_.control_block().items_count, memory2.control_block().items_count);
  EXPECT_TRUE(std::ranges::equal(memory2, memory_));
}

TEST_F(MemoryWithItemCountFixture, CopyOperator) {
  // Arrange
  memory_.resize_to_fit_at_least(1);
  std::iota(memory_.begin(), memory_.end(), uint8_t{});
  memory_.control_block().items_count = 100;

  // Act
  decltype(memory_) memory2;
  memory2.resize_to_fit_at_least(1);
  memory2 = memory_;

  // Assert
  EXPECT_NE(memory2, memory_);
  ASSERT_EQ(memory_.size(), memory2.size());
  ASSERT_EQ(memory_.control_block().data_size, memory2.control_block().data_size);
  ASSERT_EQ(memory_.control_block().items_count, memory2.control_block().items_count);
  EXPECT_TRUE(std::ranges::equal(memory2, memory_));
}

class SharedPtrFixture : public ::testing::Test {
 protected:
  template <class T>
  using SharedPtr = BareBones::SharedPtr<T, SharedPtrControlBlockWithItemCount, DefaultReallocator>;
};

TEST_F(SharedPtrFixture, Empty) {
  // Arrange

  // Act
  const SharedPtr<uint8_t> ptr;

  // Assert
  EXPECT_TRUE(ptr.is_unique());
  EXPECT_EQ(0U, ptr.use_count());
  EXPECT_EQ(nullptr, ptr.get());
}

TEST_F(SharedPtrFixture, ConstructorWithSize) {
  // Arrange

  // Act
  const SharedPtr<uint8_t> ptr(16, 0);

  // Assert
  EXPECT_TRUE(ptr.is_unique());
  EXPECT_EQ(1U, ptr.use_count());
  EXPECT_NE(nullptr, ptr.get());
}

TEST_F(SharedPtrFixture, Reallocate) {
  // Arrange
  SharedPtr<uint8_t> ptr(16, 0);

  // Act
  ptr.reallocate(0, 32);
  ptr.get()[31] = '\x00';

  // Assert
  EXPECT_TRUE(ptr.is_unique());
  EXPECT_EQ(1U, ptr.use_count());
}

template <size_t kOldAllocationSize>
struct MovingReallocator : DefaultReallocator {
  static void* reallocate(void* memory, size_t size) {
    auto* result = allocate(size);
    if (result != nullptr && memory != nullptr) {
      std::memcpy(result, memory, std::min(size, kOldAllocationSize));
      free(memory);
    }
    return result;
  }
};

TEST_F(SharedPtrFixture, MovingReallocatePreservesMetadataAndPayload) {
  // Arrange
  constexpr std::array<uint8_t, 4> kPayload{10, 20, 30, 40};
  constexpr uint32_t kItemCount = 3;
  using ControlBlock = BareBones::AtomicSharedPtrControlBlockWithItemCount;
  using Reallocator = MovingReallocator<sizeof(ControlBlock) + kPayload.size()>;
  BareBones::SharedPtr<uint8_t, ControlBlock, Reallocator> ptr(kPayload.size(), kItemCount);
  std::memcpy(ptr.get(), kPayload.data(), kPayload.size());
  const auto original_address = reinterpret_cast<uintptr_t>(ptr.get());

  // Act
  ptr.reallocate(kPayload.size(), 32);

  // Assert
  EXPECT_NE(original_address, reinterpret_cast<uintptr_t>(ptr.get()));
  EXPECT_EQ(1U, ptr.use_count());
  EXPECT_EQ(kItemCount, ptr.items_count());
  EXPECT_TRUE(std::equal(kPayload.begin(), kPayload.end(), ptr.get()));
}

TEST_F(SharedPtrFixture, CopyConstructor) {
  // Arrange
  const SharedPtr<uint8_t> ptr(16, 0);

  // Act
  // NOLINTNEXTLINE(performance-unnecessary-copy-initialization)
  const auto ptr2 = ptr;

  // Assert
  EXPECT_FALSE(ptr.is_unique());
  EXPECT_FALSE(ptr2.is_unique());
  EXPECT_EQ(2U, ptr.use_count());
  EXPECT_EQ(ptr.get(), ptr2.get());
}

TEST_F(SharedPtrFixture, CopyOperator) {
  // Arrange
  const SharedPtr<uint8_t> ptr(16, 0);

  // Act
  SharedPtr<uint8_t> ptr2(16, 0);
  ptr2 = ptr;

  // Assert
  EXPECT_FALSE(ptr.is_unique());
  EXPECT_FALSE(ptr2.is_unique());
  EXPECT_EQ(2U, ptr.use_count());
  EXPECT_EQ(ptr.get(), ptr2.get());
}

struct CountingReallocator : DefaultReallocator {
  static inline std::atomic_uint32_t frees{};

  static void free(void* memory) {
    ++frees;
    DefaultReallocator::free(memory);
  }
};

TEST_F(SharedPtrFixture, ConcurrentRelease) {
  // Arrange
  BareBones::SharedPtr<uint8_t, BareBones::AtomicSharedPtrControlBlockWithItemCount, CountingReallocator> ptr(16, 0);
  std::barrier release(2);

  // Act
  std::jthread owner([copy = ptr, &release]() mutable {
    release.arrive_and_wait();
    copy.clear();
  });
  release.arrive_and_wait();
  ptr.clear();
  owner.join();

  // Assert
  EXPECT_EQ(1U, CountingReallocator::frees.exchange(0));
}

TEST_F(SharedPtrFixture, MoveConstructor) {
  // Arrange
  SharedPtr<uint8_t> ptr(16, 0);

  // Act
  const auto ptr2 = std::move(ptr);

  // Assert
  EXPECT_EQ(1U, ptr2.use_count());
  EXPECT_NE(nullptr, ptr2.get());
}

TEST_F(SharedPtrFixture, MoveOperator) {
  // Arrange
  SharedPtr<uint8_t> ptr(16, 0);
  const auto ptr_memory = ptr.get();

  // Act
  SharedPtr<uint8_t> ptr2(16, 0);
  ptr2 = std::move(ptr);

  // Assert
  EXPECT_EQ(1U, ptr2.use_count());
  EXPECT_EQ(ptr_memory, ptr2.get());
}

TEST_F(SharedPtrFixture, ReallocateToLagerSize) {
  // Arrange
  SharedPtr<std::string_view> ptr(1, 0);
  std::construct_at(ptr.get(), "123456");
  ptr.set_items_count(1);

  // Act
  ptr.reallocate(1, 2);

  // Assert
  EXPECT_EQ(1U, ptr.items_count());
  EXPECT_EQ("123456", ptr.get()[0]);
}

TEST_F(SharedPtrFixture, ReallocateToZeroSize) {
  // Arrange
  SharedPtr<std::string_view> ptr(1, 0);
  std::construct_at(ptr.get());

  // Act
  ptr.reallocate(1, 0);

  // Assert
  EXPECT_TRUE(ptr.is_unique());
}

TEST_F(SharedPtrFixture, ReallocateToSmallerSize) {
  // Arrange
  SharedPtr<std::string_view> ptr(2, 0);
  std::construct_at(&ptr.get()[0], "123456");
  std::construct_at(&ptr.get()[1], "654321");

  // Act
  ptr.reallocate(2, 1);

  // Assert
  EXPECT_EQ("123456", ptr.get()[0]);
}

TEST_F(SharedPtrFixture, ReallocateWithCopiesToSmallerSize) {
  // Arrange
  // NOLINTNEXTLINE(clang-analyzer-unix.Malloc)
  SharedPtr<std::string_view> ptr(2, 0);
  std::construct_at(&ptr.get()[0], "123456");
  std::construct_at(&ptr.get()[1], "654321");
  ptr.set_items_count(2);

  SharedPtr<std::string_view> ptr1 = ptr;
  SharedPtr<std::string_view> ptr2 = ptr;

  // Act
  ptr.reallocate(2, 1);

  // Assert
  EXPECT_EQ("123456", ptr.get()[0]);
}

TEST_F(SharedPtrFixture, ReallocateWithCopiesToBiggerSize) {
  // Arrange
  SharedPtr<std::string_view> ptr(2, 0);
  std::construct_at(&ptr.get()[0], "123456");
  std::construct_at(&ptr.get()[1], "654321");
  ptr.set_items_count(2);

  SharedPtr<std::string_view> ptr1 = ptr;
  SharedPtr<std::string_view> ptr2 = ptr;

  // Act
  ptr.reallocate(2, 3);
  std::construct_at(&ptr.get()[2], "000000");

  // Assert
  EXPECT_EQ("123456", ptr.get()[0]);
  EXPECT_EQ("654321", ptr.get()[1]);
  EXPECT_EQ("000000", ptr.get()[2]);
  EXPECT_EQ(2, ptr.items_count());
}

class SharedMemoryFixture : public ::testing::Test {
 protected:
  SharedMemory<uint8_t, SharedPtrControlBlockWithItemCount, DefaultReallocator> memory_;
};

TEST_F(SharedMemoryFixture, Empty) {
  // Arrange

  // Act

  // Assert
  EXPECT_EQ(0U, memory_.size());
  EXPECT_EQ(nullptr, memory_);
}

TEST_F(SharedMemoryFixture, CopyConstructor) {
  // Arrange
  memory_.resize_to_fit_at_least(1);

  // Act
  const auto memory2 = memory_;

  // Assert
  EXPECT_EQ(memory2, memory_);
}

TEST_F(SharedMemoryFixture, CopyOperator) {
  // Arrange
  memory_.resize_to_fit_at_least(1);

  // Act
  SharedMemory<uint8_t, SharedPtrControlBlockWithItemCount, DefaultReallocator> memory2;
  memory2.resize_to_fit_at_least(1);
  memory2 = memory_;

  // Assert
  EXPECT_EQ(memory2, memory_);
}

TEST_F(SharedMemoryFixture, MoveConstructor) {
  // Arrange
  memory_.resize_to_fit_at_least(1);
  const auto memory_ptr = memory_.begin();
  const auto memory_size = memory_.size();

  // Act
  const auto memory2 = std::move(memory_);

  // Assert
  EXPECT_EQ(0U, memory_.size());
  EXPECT_EQ(nullptr, memory_);

  EXPECT_EQ(memory_ptr, memory2);
  EXPECT_EQ(memory_size, memory2.size());
}

TEST_F(SharedMemoryFixture, MoveOperator) {
  // Arrange
  memory_.resize_to_fit_at_least(1);
  const auto memory_ptr = memory_.begin();
  const auto memory_size = memory_.size();

  // Act
  SharedMemory<uint8_t, SharedPtrControlBlockWithItemCount, DefaultReallocator> memory2;
  memory2.resize_to_fit_at_least(1);
  memory2 = std::move(memory_);

  // Assert
  EXPECT_EQ(0U, memory_.size());
  EXPECT_EQ(nullptr, memory_);

  EXPECT_EQ(memory_ptr, memory2);
  EXPECT_EQ(memory_size, memory2.size());
}

TEST_F(SharedMemoryFixture, ResizeOnNonUniqueOwner) {
  // Arrange
  memory_.resize_to_fit_at_least(1);
  memory_.set_items_count(1);
  auto memory2 = memory_;

  // Act
  memory2.grow_to_fit_at_least(memory2.size() + 1);

  // Assert
  EXPECT_NE(memory_.size(), memory2.size());
  EXPECT_NE(memory_.begin(), memory2.begin());
  EXPECT_EQ(1U, memory2.items_count());
}

}  // namespace