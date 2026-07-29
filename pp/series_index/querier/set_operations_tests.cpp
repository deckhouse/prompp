#include <gtest/gtest.h>

#include <vector>

#include "primitives/label_set.h"
#include "series_index/querier/set_operations.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using series_index::SeriesIdSequence;
using series_index::SeriesIdSequenceSnapshot;
using series_index::querier::MatchesMerger;
using series_index::querier::Selector;
using series_index::querier::SeriesIdSpan;
using series_index::querier::SeriesSliceList;
using series_index::querier::SetSubstractor;

struct MatchesMergerCase {
  std::vector<std::vector<uint32_t>> matches;
  std::vector<uint32_t> expected;
};

class MatchesMergerFixture : public testing::TestWithParam<MatchesMergerCase> {
 protected:
  Selector<SeriesIdSequenceSnapshot>::Matcher::Matches make_matches(const std::vector<std::vector<uint32_t>>& raw_matches) {
    Selector<SeriesIdSequenceSnapshot>::Matcher::Matches matches;

    sequences_.reserve(raw_matches.size());
    for (const auto& ids : raw_matches) {
      auto& sequence = sequences_.emplace_back();
      for (const auto id : ids) {
        sequence.push_back(id);
      }
      matches.emplace_back(sequence);
    }

    return matches;
  }

 private:
  std::vector<SeriesIdSequence> sequences_;
};

TEST_P(MatchesMergerFixture, Test) {
  // Arrange
  const auto matches = make_matches(GetParam().matches);
  std::vector<uint32_t> memory(GetParam().expected.size());
  MatchesMerger merger;

  // Act
  auto result = merger.merge(matches, memory.data());

  // Assert
  EXPECT_TRUE(std::ranges::equal(GetParam().expected, result));
}

INSTANTIATE_TEST_SUITE_P(TestCases,
                         MatchesMergerFixture,
                         testing::Values(MatchesMergerCase{.matches = {}, .expected = {}},
                                         MatchesMergerCase{.matches = {{}}, .expected = {}},
                                         MatchesMergerCase{.matches = {{0, 1, 2, 3}}, .expected = {0, 1, 2, 3}},
                                         MatchesMergerCase{.matches = {{0, 1, 2}, {3, 4, 5}}, .expected = {0, 1, 2, 3, 4, 5}},
                                         MatchesMergerCase{.matches = {{3, 4, 5}, {0, 1, 2}}, .expected = {0, 1, 2, 3, 4, 5}},
                                         MatchesMergerCase{.matches = {{0, 2, 4}, {1, 3, 5}}, .expected = {0, 1, 2, 3, 4, 5}},
                                         MatchesMergerCase{.matches = {{0, 1, 2, 3}, {2, 3, 4, 5}}, .expected = {0, 1, 2, 3, 4, 5}},
                                         MatchesMergerCase{.matches = {{0, 1, 2}, {0, 1, 2}}, .expected = {0, 1, 2}},
                                         MatchesMergerCase{.matches = {{}, {0, 1, 2}, {}}, .expected = {0, 1, 2}},
                                         MatchesMergerCase{.matches = {{0, 5, 10}, {1, 5, 11}, {2, 10, 12}}, .expected = {0, 1, 2, 5, 10, 11, 12}}));

struct SetSubstracterCase {
  std::vector<uint32_t> set1;
  std::vector<uint32_t> set2;
  std::vector<uint32_t> expected;
};

class SetSubstractorFixture : public testing::TestWithParam<SetSubstracterCase> {};

TEST_P(SetSubstractorFixture, Test) {
  // Arrange
  auto set1 = GetParam().set1;

  // Act
  auto result = SetSubstractor::substract(SeriesIdSpan(set1.data(), set1.size()), GetParam().set2);

  // Assert
  EXPECT_TRUE(std::ranges::equal(GetParam().expected, result));
}

INSTANTIATE_TEST_SUITE_P(TestCases,
                         SetSubstractorFixture,
                         testing::Values(SetSubstracterCase{.set1 = {}, .set2 = {0, 1, 2}, .expected = {}},
                                         SetSubstracterCase{.set1 = {0, 1, 2, 3}, .set2 = {}, .expected = {0, 1, 2, 3}},
                                         SetSubstracterCase{.set1 = {0, 1, 2, 3}, .set2 = {4}, .expected = {0, 1, 2, 3}},
                                         SetSubstracterCase{.set1 = {0, 1, 2, 3}, .set2 = {0, 1, 2, 3}, .expected = {}},
                                         SetSubstracterCase{.set1 = {0, 1, 2, 3}, .set2 = {0, 1, 2, 3}, .expected = {}},
                                         SetSubstracterCase{.set1 = {5, 6, 7}, .set2 = {0, 1, 2, 3, 4, 5}, .expected = {6, 7}}));

}  // namespace
