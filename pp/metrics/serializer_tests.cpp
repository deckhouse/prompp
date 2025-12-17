#include <gtest/gtest.h>

#include "counter.h"
#include "serializer.h"

#if 0
namespace {

using metrics::Counter;
using metrics::LabelSet;
using metrics::Metric;
using metrics::Serializer;
using PromPP::Primitives::LabelViewSet;
using std::operator""sv;

class SerializeFixture : public testing::Test {
 protected:
  Serializer serializer_;
};

TEST_F(SerializeFixture, SerializeCounter) {
  // Arrange
  const Counter counter{"my_counter", 1234};
  const LabelSet label_set(LabelViewSet{{"__name__", "__value__"}, {"name2", "value2"}});

  // Act
  const auto& buffer = serializer_.serialize(label_set, &counter);

  // Assert
  EXPECT_EQ(
      "\x0A\x16\x0A\x08\x5F\x5F\x6E\x61\x6D\x65\x5F\x5F\x12\x0A\x6D\x79\x5F\x63\x6F\x75\x6E\x74\x65\x72\x0A\x0F\x0A\x05\x6E\x61\x6D\x65\x32\x12\x06\x76\x61\x6C"
      "\x75\x65\x32\x1A\x09\x09\x00\x00\x00\x00\x00\x48\x93\x40"sv,
      buffer);
}

}  // namespace
#endif