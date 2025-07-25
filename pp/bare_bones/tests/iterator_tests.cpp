#include <gtest/gtest.h>

#include "bare_bones/iterator.h"

namespace {

class BatchIteratorFixture : public ::testing::Test {
 protected:
  using Container = std::vector<uint32_t>;
  using Iterator = BareBones::iterator::BatchIterator<Container::const_iterator, Container::const_iterator>;

  std::vector<uint32_t> container_;
};

TEST_F(BatchIteratorFixture, TestEmptyContainer) {
  // Arrange
  const Iterator it(container_.begin(), 1);

  // Act

  // Assert
  EXPECT_EQ(it, container_.end());
}

TEST_F(BatchIteratorFixture, TestOneItem) {
  // Arrange
  container_.push_back(1);
  const Iterator it(container_.begin(), 1);

  // Act

  // Assert
  EXPECT_TRUE(std::ranges::equal(Container{1U}, std::ranges::subrange(it, container_.end())));
}

TEST_F(BatchIteratorFixture, TestBatchSize) {
  // Arrange
  container_ = {1, 2, 3, 4};
  const Iterator it(container_.begin(), 3);

  // Act

  // Assert
  EXPECT_TRUE(std::ranges::equal(Container{1U, 2U, 3U}, std::ranges::subrange(it, container_.end())));
}

TEST_F(BatchIteratorFixture, TestNextBatch) {
  // Arrange
  container_ = {1, 2, 3, 4};
  Iterator it(container_.begin(), 3);

  // Act
  std::advance(it, 3);
  it.next_batch();

  // Assert
  EXPECT_TRUE(std::ranges::equal(Container{4U}, std::ranges::subrange(it, container_.end())));
}

}  // namespace