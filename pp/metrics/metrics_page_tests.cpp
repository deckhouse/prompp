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

}  // namespace