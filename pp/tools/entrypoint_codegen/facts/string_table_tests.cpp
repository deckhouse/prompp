#include "facts/string_table.h"

#include <gtest/gtest.h>

#include <string_view>
#include <utility>

namespace {

using epgen::facts::StringId;
using epgen::facts::StringTable;

TEST(StringTableTest, StartsEmpty) {
  // Arrange
  const StringTable table;

  // Assert
  EXPECT_TRUE(table.empty());
  EXPECT_EQ(table.size(), 0);
}

TEST(StringTableTest, StoresStringsByStableId) {
  // Arrange
  StringTable table;

  // Act
  const auto first = table.add("prompp_fn");
  const auto second = table.add("entrypoint.cpp");

  // Assert
  EXPECT_FALSE(table.empty());
  EXPECT_EQ(table.size(), 2);
  EXPECT_EQ(table.get(first), "prompp_fn");
  EXPECT_EQ(table.get(second), "entrypoint.cpp");
}

TEST(StringTableTest, StoresEmptyAndRepeatedStringsAsDistinctEntries) {
  // Arrange
  StringTable table;

  // Act
  const auto empty = table.add("");
  const auto first = table.add("same");
  const auto second = table.add("same");

  // Assert
  EXPECT_EQ(table.size(), 3);
  EXPECT_EQ(table.get(empty), "");
  EXPECT_EQ(table.get(first), "same");
  EXPECT_EQ(table.get(second), "same");
  EXPECT_NE(first, second);
}

TEST(StringTableTest, PreservesEmbeddedNullBytes) {
  // Arrange
  StringTable table;
  constexpr char value[] = {'a', '\0', 'b'};

  // Act
  const auto id = table.add(std::string_view(value, sizeof(value)));

  // Assert
  EXPECT_EQ(table.get(id), std::string_view(value, sizeof(value)));
}

TEST(StringTableTest, ResolvesInvalidStringIdToPlaceholder) {
  // Arrange
  const StringTable table;
  const StringId id;

  // Act
  const std::string_view value = table.get(id);

  // Assert
  EXPECT_EQ(value, epgen::facts::kInvalidValuePlaceholder);
}

TEST(StringTableTest, MoveTransfersStoredStrings) {
  // Arrange
  StringTable table;
  const auto id = table.add("prompp_fn");

  // Act
  StringTable moved = std::move(table);

  // Assert
  EXPECT_EQ(moved.size(), 1);
  EXPECT_EQ(moved.get(id), "prompp_fn");
}

}  // namespace
