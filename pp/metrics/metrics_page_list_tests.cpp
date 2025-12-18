#include <thread>

#include <gtest/gtest.h>

#include "metrics_page_list.h"

namespace {

using metrics::Counter;
using metrics::Metric;
using metrics::MetricsPage;
using metrics::MetricsPageControlBlock;
using metrics::MetricsPageList;
using PromPP::Primitives::LabelViewSet;

class MetricsPageListFixture : public ::testing::Test {
 protected:
  using MetricsPagesVector = std::vector<MetricsPageControlBlock*>;

  struct Metrics final : MetricsPage<Metrics> {
    using MetricsPage::MetricsPage;

    Counter uint64_counter{LabelViewSet{}, "uint16_counter", 16};
  };

  MetricsPageList metrics_page_list_;

  void add_metrics_pages(const MetricsPagesVector& pages) {
    for (const auto page : pages) {
      metrics_page_list_.add(page);
    }
  }
};

TEST_F(MetricsPageListFixture, TestIteratorInEmptyList) {
  // Arrange
  MetricsPagesVector actual;

  // Act
  std::ranges::copy(metrics_page_list_, std::back_inserter(actual));

  // Assert
  EXPECT_TRUE(actual.empty());
}

TEST_F(MetricsPageListFixture, TestIteratorWithUsedPages) {
  // Arrange
  MetricsPagesVector metrics_pages{new Metrics(), new Metrics()};
  add_metrics_pages(metrics_pages);

  MetricsPagesVector actual;

  // Act
  std::ranges::copy(metrics_page_list_, std::back_inserter(actual));

  // Assert
  std::ranges::reverse(metrics_pages);
  EXPECT_EQ(metrics_pages, actual);
}

TEST_F(MetricsPageListFixture, TestIteratorWithUnusedPages) {
  // Arrange
  const MetricsPagesVector metrics_pages{new Metrics(), new Metrics(), new Metrics(), new Metrics()};
  metrics_pages[0]->detach();
  metrics_pages[1]->detach();
  metrics_pages[3]->detach();

  add_metrics_pages(metrics_pages);

  MetricsPagesVector actual;

  // Act
  std::ranges::copy(metrics_page_list_, std::back_inserter(actual));

  // Assert
  EXPECT_EQ((MetricsPagesVector{metrics_pages[2]}), actual);
}

class MetricsPageListRemoveUnusedPagesFixture : public MetricsPageListFixture {
 protected:
  MetricsPagesVector fill_4_metric_pages() {
    MetricsPagesVector pages{new Metrics(), new Metrics(), new Metrics(), new Metrics()};
    add_metrics_pages(pages);
    return pages;
  }
};

TEST_F(MetricsPageListRemoveUnusedPagesFixture, TestRemoveInEmptyList) {
  // Arrange

  // Act
  metrics_page_list_.remove_unused_pages();

  // Assert
}

TEST_F(MetricsPageListRemoveUnusedPagesFixture, TestRemoveWithoutUnusedPages) {
  // Arrange
  MetricsPagesVector metrics_pages{new Metrics(), new Metrics()};
  add_metrics_pages(metrics_pages);

  MetricsPagesVector actual;

  // Act
  metrics_page_list_.remove_unused_pages();

  // Assert
  std::ranges::copy(metrics_page_list_, std::back_inserter(actual));
  std::ranges::reverse(metrics_pages);

  EXPECT_EQ(metrics_pages, actual);
}

TEST_F(MetricsPageListRemoveUnusedPagesFixture, TestRemoveFirstMetricsPageInOnePageList) {
  // Arrange
  const auto metric = new Metrics();
  metrics_page_list_.add(metric);
  metric->detach();

  MetricsPagesVector actual;

  // Act
  metrics_page_list_.remove_unused_pages();

  // Assert
  std::ranges::copy(metrics_page_list_, std::back_inserter(actual));
  EXPECT_TRUE(actual.empty());
}

TEST_F(MetricsPageListRemoveUnusedPagesFixture, TestRemoveAllMetricsPages) {
  // Arrange
  auto metrics_pages = fill_4_metric_pages();
  std::ranges::for_each(metrics_pages, [&](auto metric) { metric->detach(); });

  MetricsPagesVector actual;

  // Act
  metrics_page_list_.remove_unused_pages();

  // Assert
  std::ranges::copy(metrics_page_list_, std::back_inserter(actual));
  EXPECT_TRUE(actual.empty());
}

TEST_F(MetricsPageListRemoveUnusedPagesFixture, TestRemoveFirstMetric) {
  // Arrange
  auto metrics_pages = fill_4_metric_pages();
  metrics_pages[0]->detach();
  metrics_pages.erase(metrics_pages.begin());

  MetricsPagesVector actual;

  // Act
  metrics_page_list_.remove_unused_pages();

  // Assert
  std::ranges::copy(metrics_page_list_, std::back_inserter(actual));
  std::ranges::reverse(metrics_pages);

  EXPECT_EQ(metrics_pages, actual);
}

TEST_F(MetricsPageListRemoveUnusedPagesFixture, TestRemoveSecondMetric) {
  // Arrange
  auto metrics_pages = fill_4_metric_pages();
  metrics_pages[1]->detach();
  metrics_pages.erase(metrics_pages.begin() + 1);

  MetricsPagesVector actual;

  // Act
  metrics_page_list_.remove_unused_pages();

  // Assert
  std::ranges::copy(metrics_page_list_, std::back_inserter(actual));
  std::ranges::reverse(metrics_pages);

  EXPECT_EQ(metrics_pages, actual);
}

TEST_F(MetricsPageListRemoveUnusedPagesFixture, TestRemoveThirdMetric) {
  // Arrange
  auto metrics_pages = fill_4_metric_pages();
  metrics_pages[2]->detach();
  metrics_pages.erase(metrics_pages.begin() + 2);

  MetricsPagesVector actual;

  // Act
  metrics_page_list_.remove_unused_pages();

  // Assert
  std::ranges::copy(metrics_page_list_, std::back_inserter(actual));
  std::ranges::reverse(metrics_pages);

  EXPECT_EQ(metrics_pages, actual);
}

TEST_F(MetricsPageListRemoveUnusedPagesFixture, TestRemoveSecondAndThirdMetric) {
  // Arrange
  auto metrics_pages = fill_4_metric_pages();
  metrics_pages[1]->detach();
  metrics_pages[2]->detach();
  metrics_pages.erase(metrics_pages.begin() + 1, metrics_pages.begin() + 3);

  MetricsPagesVector actual;

  // Act
  metrics_page_list_.remove_unused_pages();

  // Assert
  std::ranges::copy(metrics_page_list_, std::back_inserter(actual));
  std::ranges::reverse(metrics_pages);

  EXPECT_EQ(metrics_pages, actual);
}

class MetricsPageListThreadSafetyFixture : public MetricsPageListFixture {
  ;
};

TEST_F(MetricsPageListThreadSafetyFixture, DISABLED_TestAdd) {
  // Arrange
  const auto kThreadsCount = std::thread::hardware_concurrency();
  static constexpr auto kThreadTasks = 100000ULL;
  static constexpr auto kWaitThreadCreationDuration = std::chrono::milliseconds(1000);

  MetricsPagesVector pages(kThreadsCount * kThreadTasks);
  std::ranges::generate(pages, [] { return new Metrics(); });

  std::vector<std::jthread> threads_list;
  threads_list.reserve(kThreadsCount);

  // Act
  for (uint32_t i = 0; i < kThreadsCount; ++i) {
    threads_list.emplace_back([i, &pages, this] {
      std::this_thread::sleep_for(kWaitThreadCreationDuration);

      for (uint32_t offset = i * kThreadTasks, counter = 0; counter < kThreadTasks; ++counter) {
        metrics_page_list_.add(pages[offset + counter]);
      }
    });
  }

  // Assert
}

}  // namespace