#include <gtest/gtest.h>

#include <algorithm>

#include "series_index/trie/cedarpp_tree.h"

namespace {

using series_index::trie::CedarTrie;
using std::operator""sv;
using std::operator""s;

struct TrieItem {
  std::string key;
  uint32_t value;

  bool operator==(const TrieItem&) const noexcept = default;
  bool operator<(const TrieItem& other) const noexcept { return key < other.key; }
};

class CedarTrieFixture {
 protected:
  CedarTrie trie_;

  [[nodiscard]] static std::vector<TrieItem> items(const CedarTrie& trie) {
    std::vector<TrieItem> actual;
    for (auto it = trie.make_enumerative_iterator(); it.is_valid(); ++it) {
      actual.emplace_back(TrieItem{.key = std::string(it.key()), .value = it.value()});
    }
    return actual;
  }
};

class CedarTrieZeroByteSupportFixture : public CedarTrieFixture, public testing::Test {};

TEST_F(CedarTrieZeroByteSupportFixture, InsertAndLookupStringWithZeroBytes) {
  // Arrange
  static constexpr auto kKey = "HetznerFinland:\x00:nova"sv;
  static constexpr auto kValue = 123U;

  // Act
  trie_.insert(kKey, kValue);
  const auto value1 = trie_.lookup(kKey);
  const auto value2 = trie_.lookup("HetznerFinland:");

  // Assert
  EXPECT_EQ(kValue, value1.value_or(0));
  EXPECT_FALSE(value2);
}

struct CedarEnumerativeIteratorCase {
  std::vector<TrieItem> items;
  std::vector<TrieItem> expected;
};

class CedarEnumerativeIteratorFixture : public CedarTrieFixture, public testing::TestWithParam<CedarEnumerativeIteratorCase> {
 protected:
  void SetUp() final {
    for (auto& item : GetParam().items) {
      trie_.insert(item.key, item.value);
    }
  }
};

TEST_P(CedarEnumerativeIteratorFixture, EnumeratesExpectedItems) {
  // Arrange

  // Act
  const auto actual = items(trie_);

  // Assert
  EXPECT_EQ(GetParam().expected, actual);
}

INSTANTIATE_TEST_SUITE_P(ValueWithZeroByte,
                         CedarEnumerativeIteratorFixture,
                         testing::Values(CedarEnumerativeIteratorCase{.items =
                                                                          {
                                                                              {.key = "HetznerFinland:\x00:nova"s, .value = 0},
                                                                              {.key = "HetznerFinland:\x00:mova"s, .value = 1},
                                                                              {.key = "HetznerAlaska:\x00:kova"s, .value = 2},
                                                                          },
                                                                      .expected =
                                                                          {
                                                                              {.key = "HetznerAlaska:\x00:kova"s, .value = 2},
                                                                              {.key = "HetznerFinland:\x00:mova"s, .value = 1},
                                                                              {.key = "HetznerFinland:\x00:nova"s, .value = 0},
                                                                          }},
                                         CedarEnumerativeIteratorCase{.items =
                                                                          {
                                                                              {.key = "\x00\x00"s, .value = 0},
                                                                              {.key = "\x00"s, .value = 1},
                                                                          },
                                                                      .expected =
                                                                          {
                                                                              {.key = "\x00"s, .value = 1},
                                                                              {.key = "\x00\x00"s, .value = 0},
                                                                          }},
                                         CedarEnumerativeIteratorCase{.items =
                                                                          {
                                                                              {.key = "\x01\x01"s, .value = 0},
                                                                              {.key = "\x01\x00"s, .value = 1},
                                                                          },
                                                                      .expected =
                                                                          {
                                                                              {.key = "\x01\x00"s, .value = 1},
                                                                              {.key = "\x01\x01"s, .value = 0},
                                                                          }},
                                         CedarEnumerativeIteratorCase{.items =
                                                                          {
                                                                              {.key = "\x01"s, .value = 0},
                                                                              {.key = "\x00"s, .value = 1},
                                                                          },
                                                                      .expected = {
                                                                          {.key = "\x00"s, .value = 1},
                                                                          {.key = "\x01"s, .value = 0},
                                                                      }}));

struct SerializeDeserializeCase {
  std::vector<TrieItem> items;
};

class CedarTrieSerializeDeserializeFixture : public CedarTrieFixture, public ::testing::TestWithParam<SerializeDeserializeCase> {
 protected:
  void SetUp() final {
    for (const auto& [key, value] : GetParam().items) {
      trie_.insert(key, value);
    }
  }
};

TEST_P(CedarTrieSerializeDeserializeFixture, RoundTripPreservesItems) {
  // Arrange
  std::stringstream stream;
  CedarTrie trie2;

  // Act
  stream << trie_;
  stream >> trie2;

  // Assert
  auto& sorted_items = const_cast<SerializeDeserializeCase&>(GetParam()).items;
  std::sort(sorted_items.begin(), sorted_items.end());
  EXPECT_EQ(GetParam().items, items(trie2));
}

INSTANTIATE_TEST_SUITE_P(EmptyTrie, CedarTrieSerializeDeserializeFixture, testing::Values(SerializeDeserializeCase{}));
INSTANTIATE_TEST_SUITE_P(Cases,
                         CedarTrieSerializeDeserializeFixture,
                         testing::Values(SerializeDeserializeCase{.items = {{.key = "key", .value = 1}}},
                                         SerializeDeserializeCase{.items = {
                                                                      {.key = "key1", .value = 1},
                                                                      {.key = "key2", .value = 2},
                                                                      {.key = "key3", .value = 3},
                                                                  }}));

}  // namespace
