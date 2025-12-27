#include <gtest/gtest.h>

#include "primitives/go_metric.h"

namespace {

using PromPP::Primitives::LabelView;
using PromPP::Primitives::Go::String;
using PromPP::Primitives::Go::dto::LabelPair;
using PromPP::Primitives::Go::dto::LabelPairsList;
using PromPP::Primitives::Go::dto::MetricDescriptor;

class MetricDescriptorFixture : public testing::Test {};

TEST_F(MetricDescriptorFixture, TestWithEmptyLabels) {
  // Arrange
  constexpr LabelPairsList labels;

  // Act
  const MetricDescriptor descriptor(String("metric_name"), labels, nullptr);

  // Assert
  EXPECT_EQ(10848170393603132573ULL, descriptor.id);
  EXPECT_EQ(10764519495013463364ULL, descriptor.dim_hash);
}

TEST_F(MetricDescriptorFixture, TestWithLabels) {
  // Arrange
  constexpr String name1{"name1"};
  constexpr String value1{"value1"};
  constexpr String name2{"name2"};
  constexpr String value2{"value2"};
  const LabelPairsList labels{LabelPair{&name2, &value2}, LabelPair{&name1, &value1}};

  // Act
  const MetricDescriptor descriptor(String("metric_name"), labels, nullptr);

  // Assert
  EXPECT_EQ(9433770049495071547ULL, descriptor.id);
  EXPECT_EQ(1413792954011449091ULL, descriptor.dim_hash);
}

}  // namespace