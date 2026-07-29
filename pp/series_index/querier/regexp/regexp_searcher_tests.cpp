#include <algorithm>
#include <cstdint>
#include <limits>
#include <ranges>
#include <string_view>
#include <tuple>
#include <vector>

#include <gtest/gtest.h>

#include "series_index/querier/regexp/regexp.h"
#include "series_index/querier/regexp/regexp_searcher.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using series_index::querier::ValueMatchIdResolver;
using series_index::querier::regexp::RegexpParser;
using series_index::querier::regexp::RegexpSearcher;
using series_index::trie::CedarMatchesList;
using series_index::trie::CedarTrie;
using std::operator""sv;

struct RegexpSearcherTestCase {
  std::vector<std::string_view> trie_values;
  std::string_view regexp;
  std::vector<std::string_view> matches;
};

class CedarTrieRegexpSearcherFixture : public testing::TestWithParam<RegexpSearcherTestCase> {
 protected:
  using MatchesList = std::vector<uint32_t>;

  CedarTrie trie_;
  MatchesList matches_;
  CedarMatchesList<MatchesList, ValueMatchIdResolver> matches_list_{matches_, {}};
  RegexpSearcher<CedarTrie, decltype(matches_list_)> searcher_{matches_list_};

  void SetUp() final {
    uint32_t id = 0;
    for (auto& key : GetParam().trie_values) {
      trie_.insert(key, id++);
    }
  }

  [[nodiscard]] MatchesList get_expected_matches() const {
    MatchesList expected_matches;
    for (auto& key : GetParam().matches) {
      expected_matches.push_back(trie_.lookup(key).value_or(std::numeric_limits<uint32_t>::max()));
    }

    return expected_matches;
  }
};

TEST_P(CedarTrieRegexpSearcherFixture, FindsExpectedMatches) {
  // Arrange
  auto expected_matches = get_expected_matches();

  // Act
  std::ignore = searcher_.search(trie_, RegexpParser::parse(GetParam().regexp));

  // Assert
  std::ranges::sort(expected_matches);
  std::ranges::sort(matches_);
  EXPECT_EQ(expected_matches, matches_);
}

INSTANTIATE_TEST_SUITE_P(
    SearchByPrefix,
    CedarTrieRegexpSearcherFixture,
    testing::Values(RegexpSearcherTestCase{.trie_values = {"abc", "abd"}, .regexp = "abcd", .matches = {}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "abd"}, .regexp = "ac", .matches = {}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "cba"}, .regexp = "abc", .matches = {"abc"}},
                    RegexpSearcherTestCase{.trie_values = {"a\000c"sv, "a\000d"sv}, .regexp = R"(a\x00c)", .matches = {"a\000c"sv}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "abcd"}, .regexp = "abc(bc|cd){0}", .matches = {"abc"}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "cba"}, .regexp = "abc(bc|cd){0}", .matches = {"abc"}},
                    RegexpSearcherTestCase{.trie_values = {"a\000c"sv, "a\000d"sv}, .regexp = R"(a\x00(c|d))", .matches = {"a\000c"sv, "a\000d"sv}},
                    RegexpSearcherTestCase{.trie_values = {"a\000c"sv, "a\000d"sv}, .regexp = R"(a\x00c(b|d){0})", .matches = {"a\000c"sv}},
                    RegexpSearcherTestCase{.trie_values = {"abcd", "abcd-1", "abcd-2"}, .regexp = "abcd", .matches = {"abcd"}},
                    RegexpSearcherTestCase{.trie_values = {"abcde-1", "abcde-2"}, .regexp = "abcd", .matches = {}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "acd"}, .regexp = "a", .matches = {}}));

INSTANTIATE_TEST_SUITE_P(
    SearchAlternatives,
    CedarTrieRegexpSearcherFixture,
    testing::Values(RegexpSearcherTestCase{.trie_values = {"abc", "cba", "cbb"}, .regexp = "abc|cba", .matches = {"abc", "cba"}},
                    RegexpSearcherTestCase{.trie_values = {"\x00"sv, "\x00\x00"sv}, .regexp = R"((\x00|\x00\x00))", .matches = {"\x00"sv, "\x00\x00"sv}}));

INSTANTIATE_TEST_SUITE_P(
    SearchCharacterClass,
    CedarTrieRegexpSearcherFixture,
    testing::Values(RegexpSearcherTestCase{.trie_values = {"abc", "abd", "abb"}, .regexp = "ab[cd]", .matches = {"abc", "abd"}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "acd"}, .regexp = "ab[cd]", .matches = {"abc"}},
                    RegexpSearcherTestCase{.trie_values = {"ab\x00"sv, "acd"}, .regexp = "ab[\000d]"sv, .matches = {"ab\x00"sv}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "abcd"}, .regexp = "ab[cd]$", .matches = {"abc"}},
                    RegexpSearcherTestCase{.trie_values = {"ab\x00"sv, "abcd"}, .regexp = "ab[\000d]$"sv, .matches = {"ab\x00"sv}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "acd"}, .regexp = "ad[cd]", .matches = {}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "acd"}, .regexp = "a[^0-9]", .matches = {}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "acd", "a\x00"sv}, .regexp = "a[^0-9].*", .matches = {"abc", "acd", "a\x00"sv}},
                    RegexpSearcherTestCase{.trie_values = {"a_bc", "a_bc_", "a_bc_0", "a_bc_1", "a_cd", "a_cd_", "a_cd_0"},
                                           .regexp = "a_(bc|cd)_.*",
                                           .matches = {"a_bc_", "a_bc_0", "a_bc_1", "a_cd_", "a_cd_0"}}));

INSTANTIATE_TEST_SUITE_P(
    SearchByPrefixAndRegexp,
    CedarTrieRegexpSearcherFixture,
    testing::Values(RegexpSearcherTestCase{.trie_values = {"abc"}, .regexp = "ab.*", .matches = {"abc"}},
                    RegexpSearcherTestCase{.trie_values = {"ab", "abc", "ab\x00"sv}, .regexp = "ab.*", .matches = {"ab", "abc", "ab\x00"sv}},
                    RegexpSearcherTestCase{.trie_values = {"ab"}, .regexp = "ab.*", .matches = {"ab"}},
                    RegexpSearcherTestCase{.trie_values = {"ab", "abc", "ab\x00"sv}, .regexp = "ab.+", .matches = {"abc", "ab\x00"sv}},
                    RegexpSearcherTestCase{.trie_values = {"ab", "abc"}, .regexp = "abc+", .matches = {"abc"}},
                    RegexpSearcherTestCase{.trie_values = {"ab", "abc"}, .regexp = "abc*", .matches = {"ab", "abc"}},
                    RegexpSearcherTestCase{.trie_values = {"ab"}, .regexp = "ab.+", .matches = {}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "abd", "abb"}, .regexp = "ab.*", .matches = {"abc", "abd", "abb"}},
                    RegexpSearcherTestCase{.trie_values = {"abc", "abd", "abb"}, .regexp = "abc.*", .matches = {"abc"}},
                    RegexpSearcherTestCase{.trie_values = {"ab"}, .regexp = "abc?", .matches = {"ab"}},
                    RegexpSearcherTestCase{.trie_values = {"abcde", "abcfg"}, .regexp = "abc$", .matches = {}},
                    RegexpSearcherTestCase{.trie_values = {"abcde"}, .regexp = "a.+", .matches = {"abcde"}},
                    RegexpSearcherTestCase{.trie_values = {"/frontend.Frontend/Process"},
                                           .regexp = "metrics|/frontend.Frontend/Process",
                                           .matches = {"/frontend.Frontend/Process"}}));

INSTANTIATE_TEST_SUITE_P(SearchByRegexp,
                         CedarTrieRegexpSearcherFixture,
                         testing::Values(RegexpSearcherTestCase{.trie_values = {"abc"}, .regexp = "^.{5,}", .matches = {}}));

INSTANTIATE_TEST_SUITE_P(SearchByRegexpWithEndText,
                         CedarTrieRegexpSearcherFixture,
                         testing::Values(RegexpSearcherTestCase{.trie_values = {"abc"}, .regexp = ".{5,}$", .matches = {}},
                                         RegexpSearcherTestCase{.trie_values = {"abc", "abcde"}, .regexp = ".{5,}$", .matches = {"abcde"}},
                                         RegexpSearcherTestCase{.trie_values = {"nodejs", "php", "python", "java"},
                                                                .regexp = "^(php|nodejs|python)$",
                                                                .matches = {"nodejs", "php", "python"}}));

INSTANTIATE_TEST_SUITE_P(SearchByCaseInsensitive,
                         CedarTrieRegexpSearcherFixture,
                         testing::Values(RegexpSearcherTestCase{.trie_values = {"abCdE"}, .regexp = "(?i)ABcDe", .matches = {"abCdE"}},
                                         RegexpSearcherTestCase{.trie_values = {"abCdE"}, .regexp = "(?i)ABcDe.*", .matches = {"abCdE"}},
                                         RegexpSearcherTestCase{.trie_values = {"abC\000dE\000"sv}, .regexp = "(?i)ABc.De.*", .matches = {"abC\000dE\000"sv}}));

}  // namespace
