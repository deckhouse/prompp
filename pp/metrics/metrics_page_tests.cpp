#include <gtest/gtest.h>

#include "metrics_page.h"

namespace {

using metrics::AtomicCounter;
using metrics::Counter;
using metrics::Metric;
using metrics::MetricsPage;
using PromPP::Primitives::LabelViewSet;

class MetricsPageFixture : public ::testing::Test {
 protected:
  using MetricsVector = std::vector<const Metric*>;
};

TEST_F(MetricsPageFixture, TestIterator) {
  // Arrange
  struct Metrics : MetricsPage<Metrics> {
    using MetricsPage::MetricsPage;

    Counter uint16_counter{LabelViewSet{}, "uint16_counter", 16};
    Counter uint32_counter{LabelViewSet{}, "uint32_counter", 32};
    AtomicCounter atomic_counter{LabelViewSet{}, "atomic_uint64_counter", 64};
  } const metrics;

  MetricsVector metric_pointers;

  // Act
  std::ranges::copy(metrics, std::back_inserter(metric_pointers));

  // Assert
  EXPECT_EQ((MetricsVector{&metrics.uint16_counter, &metrics.uint32_counter, &metrics.atomic_counter}), metric_pointers);
}

TEST_F(MetricsPageFixture, TestIteratorForPageWithMetdata) {
  // Arrange
  struct Metrics : MetricsPage<Metrics> {
    using MetricsPage::MetricsPage;

    Metrics() : MetricsPage{uint16_counter} {}

    std::string label_name_{"label_name"};
    
    Counter uint16_counter{LabelViewSet{{"label", label_name_}}, "uint16_counter", 16};
    Counter uint32_counter{LabelViewSet{{"label", label_name_}}, "uint32_counter", 32};
    AtomicCounter atomic_counter{LabelViewSet{{"label", label_name_}}, "atomic_uint64_counter", 64};
  } const metrics;

  MetricsVector metric_pointers;

  // Act
  std::ranges::copy(metrics, std::back_inserter(metric_pointers));

  // Assert
  EXPECT_EQ((MetricsVector{&metrics.uint16_counter, &metrics.uint32_counter, &metrics.atomic_counter}), metric_pointers);
}

}  // namespace