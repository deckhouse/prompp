#include <gtest/gtest.h>

#include "counter.h"
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

    Counter<> counter{"counter", 64};
  };

  struct Metrics2 final : MetricsPage<Metrics2> {
    using MetricsPage::MetricsPage;

    Counter<> counter1{"counter1", 64};
    Counter<> counter2{"counter2", 64};
  };
};

TEST_F(MetricsStorageIteratorFixture, EmptyStorage) {
  // Arrange
  std::vector<Storage::Iterator::Item> items;

  // Act
  std::ranges::copy(storage_, std::back_inserter(items));

  // Assert
  EXPECT_TRUE(items.empty());
}

TEST_F(MetricsStorageIteratorFixture, TwoPages) {
  // Arrange
  const auto page1 = CreateMetricsPage<Metrics1>(storage_, LabelViewSet{{"page", "1"}});
  const auto page2 = CreateMetricsPage<Metrics2>(storage_, LabelViewSet{{"page", "2"}});

  std::vector<Storage::Iterator::Item> items;

  // Act
  std::ranges::copy(storage_, std::back_inserter(items));

  // Assert
  EXPECT_TRUE(std::ranges::equal(
      std::vector<Storage::Iterator::Item>{
          {.page = page2, .metric = &page2->counter1},
          {.page = page2, .metric = &page2->counter2},
          {.page = page1, .metric = &page1->counter},
      },
      items));
}

}  // namespace