#include <gtest/gtest.h>

#include <array>
#include <cstdint>
#include <cstring>
#include <string_view>
#include <type_traits>
#include <utility>

#include "performance_tests/dummy_wal.h"

namespace {

class DummyWalFixture : public testing::Test {
 protected:
  void SetUp() override {
    store(0, int64_t{42});
    store(8, 1.5);
    store(16, uint16_t{4});
    store(18, std::array<uint16_t, 4>{3, 1, 1, 1});
    store(26, uint16_t{6});
    std::memcpy(bytes_.data() + 28, "jobaxz", 6);

    store(40, int64_t{84});
    store(48, 2.5);
    store(56, uint16_t{0});
    store(58, uint16_t{0});
  }

  template <class T>
  void store(size_t offset, const T& value) {
    static_assert(std::is_trivially_copyable_v<T>);
    std::memcpy(bytes_.data() + offset, &value, sizeof(value));
  }

  alignas(uint64_t) std::array<char, 64> bytes_{};
};

TEST_F(DummyWalFixture, ReadsSample) {
  // Arrange
  DummyWal::Timeseries series;

  // Act
  series.read_from_bytes(bytes_.data());

  // Assert
  EXPECT_EQ(42, series.samples()[0].timestamp());
  EXPECT_EQ(1.5, series.samples()[0].value());
}

TEST_F(DummyWalFixture, IteratesLabelPairsInRecordOrder) {
  // Arrange
  DummyWal::Timeseries series;
  series.read_from_bytes(bytes_.data());

  // Act
  auto it = series.label_set().begin();
  const auto first = *it;
  ++it;
  const auto second = *it;
  ++it;
  const auto at_end = it == series.label_set().end();

  // Assert
  EXPECT_EQ(2U, series.label_set().size());
  EXPECT_EQ((std::pair<std::string_view, std::string_view>{"job", "a"}), first);
  EXPECT_EQ((std::pair<std::string_view, std::string_view>{"x", "z"}), second);
  EXPECT_TRUE(at_end);
}

TEST_F(DummyWalFixture, IteratesLabelNamesWithoutValues) {
  // Arrange
  DummyWal::Timeseries series;
  series.read_from_bytes(bytes_.data());
  const auto names = series.label_set().names();

  // Act
  auto it = names.begin();
  const auto first = *it;
  ++it;
  const auto second = *it;
  ++it;
  const auto at_end = it == names.end();

  // Assert
  EXPECT_EQ(2U, names.size());
  EXPECT_EQ("job", first);
  EXPECT_EQ("x", second);
  EXPECT_TRUE(at_end);
}

TEST_F(DummyWalFixture, AdvancesAcrossEightByteRecordPadding) {
  // Arrange
  DummyWal::Timeseries first;
  DummyWal::Timeseries second;

  // Act
  char* next = first.read_from_bytes(bytes_.data());
  char* end = second.read_from_bytes(next);

  // Assert
  EXPECT_EQ(bytes_.data() + 40, next);
  EXPECT_EQ(bytes_.data() + 64, end);
  EXPECT_EQ(84, second.samples()[0].timestamp());
  EXPECT_EQ(2.5, second.samples()[0].value());
}

TEST_F(DummyWalFixture, EmptyLabelRecordHasEmptyPairAndNameRanges) {
  // Arrange
  DummyWal::Timeseries series;

  // Act
  series.read_from_bytes(bytes_.data() + 40);
  const auto& labels = series.label_set();
  const auto names = labels.names();

  // Assert
  EXPECT_EQ(0U, labels.size());
  EXPECT_TRUE(labels.begin() == labels.end());
  EXPECT_EQ(0U, names.size());
  EXPECT_TRUE(names.begin() == names.end());
}

}  // namespace
