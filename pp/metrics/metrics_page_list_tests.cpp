#include <thread>

#include <gtest/gtest.h>

#include "metrics_page_list.h"

namespace {

using metrics::Counter;
using metrics::Gauge;
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

    [[maybe_unused]] Counter uint64_counter{LabelViewSet{}, "uint16_counter", 16};
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

TEST_F(MetricsPageListFixture, TestIteratorWithAttachedPages) {
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

TEST_F(MetricsPageListFixture, TestIteratorWithDetachedPages) {
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

TEST_F(MetricsPageListFixture, TestIteratorWithRefreshableMetricsPage) {
  // Arrange
  static constexpr auto kExpectedValue = 12345;

  struct RefreshableMetrics final : MetricsPage<Metrics> {
    using MetricsPage::MetricsPage;

    void refresh_metrics() noexcept override { uint64_gauge.set(kExpectedValue); }

    Gauge uint64_gauge{LabelViewSet{}, "uint16_counter"};
  };

  auto* metrics_page = new RefreshableMetrics();
  const MetricsPagesVector metrics_pages{metrics_page};

  add_metrics_pages(metrics_pages);

  MetricsPagesVector actual;

  // Act
  std::ranges::copy(metrics_page_list_, std::back_inserter(actual));

  // Assert
  ASSERT_EQ((MetricsPagesVector{metrics_pages[0]}), actual);
  EXPECT_EQ(kExpectedValue, metrics_page->uint64_gauge.value());
}

class MetricsPageListRemoveUnusedPagesFixture : public testing::Test {
 protected:
  using RemovedPagesVector = std::vector<MetricsPageControlBlock*>;
  struct Metrics final : MetricsPage<Metrics> {
    using MetricsPage::MetricsPage;

    explicit Metrics(RemovedPagesVector& removed_pages) : MetricsPage(&Metrics::counter), removed_pages_(removed_pages) {}
    ~Metrics() override { removed_pages_.emplace_back(this); }

    RemovedPagesVector& removed_pages_;
    Counter counter{LabelViewSet{}, "counter"};
  };
  using MetricsPagesVector = std::vector<Metrics*>;

  RemovedPagesVector removed_pages_;
  MetricsPageList metrics_page_list_;

  void add_metrics_pages(const MetricsPagesVector& pages) {
    for (const auto page : pages) {
      metrics_page_list_.add(page);
    }
  }

  MetricsPagesVector fill_4_metric_pages() {
    MetricsPagesVector pages{new Metrics(removed_pages_), new Metrics(removed_pages_), new Metrics(removed_pages_), new Metrics(removed_pages_)};
    add_metrics_pages(pages);
    return pages;
  }
};

TEST_F(MetricsPageListRemoveUnusedPagesFixture, TestRemoveInEmptyList) {
  // Arrange

  // Act
  metrics_page_list_.remove_unused_pages();

  // Assert
  EXPECT_TRUE(removed_pages_.empty());
}

TEST_F(MetricsPageListRemoveUnusedPagesFixture, TestRemoveWithoutUnusedPages) {
  // Arrange
  const MetricsPagesVector metrics_pages{new Metrics(removed_pages_), new Metrics(removed_pages_)};
  add_metrics_pages(metrics_pages);

  // Act
  metrics_page_list_.remove_unused_pages();

  // Assert
  EXPECT_TRUE(removed_pages_.empty());
}

TEST_F(MetricsPageListRemoveUnusedPagesFixture, TestRemoveFirstMetricsPageInOnePageList) {
  // Arrange
  const auto metric = new Metrics(removed_pages_);
  metrics_page_list_.add(metric);
  metric->detach();
  metric->counter.deactivate();

  // Act
  metrics_page_list_.remove_unused_pages();

  // Assert
  EXPECT_EQ(RemovedPagesVector{metric}, removed_pages_);
}

TEST_F(MetricsPageListRemoveUnusedPagesFixture, TestRemoveAllMetricsPages) {
  // Arrange
  const auto metrics_pages = fill_4_metric_pages();
  std::ranges::for_each(metrics_pages, [&](auto metric) {
    metric->detach();
    metric->counter.deactivate();
  });

  // Act
  metrics_page_list_.remove_unused_pages();

  // Assert
  EXPECT_EQ(RemovedPagesVector({metrics_pages[2], metrics_pages[1], metrics_pages[0], metrics_pages[3]}), removed_pages_);
}

TEST_F(MetricsPageListRemoveUnusedPagesFixture, TestRemoveFirstMetricsPage) {
  // Arrange
  const auto metrics_pages = fill_4_metric_pages();
  metrics_pages[0]->detach();
  metrics_pages[0]->counter.deactivate();

  // Act
  metrics_page_list_.remove_unused_pages();

  // Assert
  EXPECT_EQ(RemovedPagesVector{metrics_pages[0]}, removed_pages_);
}

TEST_F(MetricsPageListRemoveUnusedPagesFixture, TestRemoveSecondMetricsPage) {
  // Arrange
  const auto metrics_pages = fill_4_metric_pages();
  metrics_pages[1]->detach();
  metrics_pages[1]->counter.deactivate();

  // Act
  metrics_page_list_.remove_unused_pages();

  // Assert
  EXPECT_EQ(RemovedPagesVector{metrics_pages[1]}, removed_pages_);
}

TEST_F(MetricsPageListRemoveUnusedPagesFixture, TestRemoveThirdMetricsPage) {
  // Arrange
  const auto metrics_pages = fill_4_metric_pages();
  metrics_pages[2]->detach();
  metrics_pages[2]->counter.deactivate();

  // Act
  metrics_page_list_.remove_unused_pages();

  // Assert
  EXPECT_EQ(RemovedPagesVector{metrics_pages[2]}, removed_pages_);
}

TEST_F(MetricsPageListRemoveUnusedPagesFixture, TestRemoveSecondAndThirdMetricsPage) {
  // Arrange
  const auto metrics_pages = fill_4_metric_pages();
  metrics_pages[1]->detach();
  metrics_pages[1]->counter.deactivate();
  metrics_pages[2]->detach();
  metrics_pages[2]->counter.deactivate();

  MetricsPagesVector actual;

  // Act
  metrics_page_list_.remove_unused_pages();

  // Assert
  EXPECT_EQ((RemovedPagesVector{metrics_pages[2], metrics_pages[1]}), removed_pages_);
}

TEST_F(MetricsPageListRemoveUnusedPagesFixture, DontRemoveDetachedActivePage) {
  // Arrange
  const auto metrics_pages = fill_4_metric_pages();
  metrics_pages[1]->detach();

  MetricsPagesVector actual;

  // Act
  metrics_page_list_.remove_unused_pages();

  // Assert
  EXPECT_TRUE(removed_pages_.empty());
}

class MetricsPageListThreadSafetyFixture : public MetricsPageListFixture {};

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