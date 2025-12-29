#include <gtest/gtest.h>

#include "storage.h"

namespace {

using metrics::Counter;
using metrics::CreateMetricsPage;
using metrics::Metric;
using metrics::MetricsPage;
using metrics::MetricsPageControlBlock;
using metrics::MetricsPageList;
using metrics::Storage;
using PromPP::Primitives::LabelViewSet;

class MetricsStorageIteratorFixture : public testing::Test {
 protected:
  Storage storage_;

  struct Metrics1 final : MetricsPage<Metrics1> {
    using MetricsPage::MetricsPage;

    Counter counter{LabelViewSet{}, "counter", 64};
  };

  struct Metrics2 final : MetricsPage<Metrics2> {
    using MetricsPage::MetricsPage;

    Counter counter1{LabelViewSet{}, "counter1", 64};
    Counter counter2{LabelViewSet{}, "counter2", 64};
  };
};

TEST_F(MetricsStorageIteratorFixture, EmptyStorage) {
  // Arrange
  std::vector<Storage::Iterator::value_type> items;

  // Act
  std::ranges::copy(storage_, std::back_inserter(items));

  // Assert
  EXPECT_TRUE(items.empty());
}

TEST_F(MetricsStorageIteratorFixture, TwoPages) {
  // Arrange
  const auto page1 = CreateMetricsPage<Metrics1>(storage_);
  const auto page2 = CreateMetricsPage<Metrics2>(storage_);

  std::vector<Storage::Iterator::value_type> items;

  // Act
  std::ranges::copy(storage_, std::back_inserter(items));

  // Assert
  EXPECT_TRUE(std::ranges::equal(
      std::vector<Storage::Iterator::value_type>{
          &page2->counter1,
          &page2->counter2,
          &page1->counter,
      },
      items));
}

}  // namespace