#include <span>
#include <vector>

#include <gtest/gtest.h>

#include "primitives/label_set.h"
#include "primitives/snug_composites.h"
#include "series_index/querier/series_operations.h"
#include "series_index/queryable_encoding_bimap.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using series_index::QueryableEncodingBimap;
using series_index::querier::group_series_by_label_names;

using Index = QueryableEncodingBimap<BareBones::Vector>;

class GroupSeriesByLabelNamesFixture : public testing::Test {
 protected:
  [[nodiscard]] uint32_t name_id(std::string_view name) { return *index_.trie_index().names_trie().lookup(name); }

  void group(const std::vector<uint32_t>& series_ids, const std::vector<uint32_t>& label_name_ids) {
    groups_.clear();
    group_series_by_label_names(index_, std::span(series_ids.data(), series_ids.size()), std::span(label_name_ids.data(), label_name_ids.size()), groups_);
  }

  Index index_;
  std::vector<std::vector<uint32_t>> groups_;
};

TEST_F(GroupSeriesByLabelNamesFixture, EmptySeriesIds) {
  // Arrange
  index_.find_or_emplace(LabelViewSet{{"j", "v"}});
  const std::vector label_name_ids{name_id("j")};

  // Act
  group(std::vector<uint32_t>{}, label_name_ids);

  // Assert
  EXPECT_TRUE(groups_.empty());
}

TEST_F(GroupSeriesByLabelNamesFixture, SingleSeriesSingleLabel) {
  // Arrange
  const auto sid = index_.find_or_emplace(LabelViewSet{{"job", "a"}, {"instance", "i"}});
  const std::vector series_ids{sid};
  const std::vector label_name_ids{name_id("job")};

  // Act
  group(series_ids, label_name_ids);

  // Assert
  ASSERT_EQ(groups_.size(), 1U);
  EXPECT_EQ((std::vector{sid}), groups_[0]);
}

TEST_F(GroupSeriesByLabelNamesFixture, TwoGroups) {
  // Arrange
  const auto s0 = index_.find_or_emplace(LabelViewSet{{"job", "j10"}, {"instance", "1"}});
  const auto s1 = index_.find_or_emplace(LabelViewSet{{"job", "j10"}, {"instance", "2"}});
  const auto s2 = index_.find_or_emplace(LabelViewSet{{"job", "j11"}, {"instance", "3"}});
  const std::vector series_ids{s0, s1, s2};
  const std::vector label_name_ids{name_id("job")};

  // Act
  group(series_ids, label_name_ids);

  // Assert
  EXPECT_EQ((std::vector{std::vector{s0, s1}, std::vector{s2}}), groups_);
}

TEST_F(GroupSeriesByLabelNamesFixture, ThreeGroups) {
  // Arrange
  const auto s0 = index_.find_or_emplace(LabelViewSet{{"n10", "v1"}, {"n20", "v2"}});
  const auto s1 = index_.find_or_emplace(LabelViewSet{{"n10", "v1"}, {"n20", "v3"}});
  const auto s2 = index_.find_or_emplace(LabelViewSet{{"n10", "v4"}, {"n20", "v2"}});
  const std::vector series_ids{s0, s1, s2};
  const std::vector label_name_ids{name_id("n10"), name_id("n20")};

  // Act
  group(series_ids, label_name_ids);

  // Assert
  EXPECT_EQ((std::vector{std::vector{s0}, std::vector{s1}, std::vector{s2}}), groups_);
}

TEST_F(GroupSeriesByLabelNamesFixture, MissingLabelUsesSentinelAndGroupsMatchingMissing) {
  // Arrange
  const auto s0 = index_.find_or_emplace(LabelViewSet{{"la", "5"}});
  const auto s1 = index_.find_or_emplace(LabelViewSet{{"lb", "9"}});
  const auto s2 = index_.find_or_emplace(LabelViewSet{{"lc", "9"}});
  const auto s3 = index_.find_or_emplace(LabelViewSet{{"ld", "9"}});
  const std::vector series_ids{s0, s1, s2, s3};
  const std::vector label_name_ids{name_id("la"), name_id("lb")};

  // Act
  group(series_ids, label_name_ids);

  // Assert
  EXPECT_EQ((std::vector{std::vector{s0}, std::vector{s1}, std::vector{s2, s3}}), groups_);
}

TEST_F(GroupSeriesByLabelNamesFixture, EmptyLabelNameIdsPutsAllSeriesInOneGroup) {
  // Arrange
  const auto s0 = index_.find_or_emplace(LabelViewSet{{"a", "1"}});
  const auto s1 = index_.find_or_emplace(LabelViewSet{{"b", "2"}});
  const std::vector series_ids{s0, s1};

  // Act
  group(series_ids, std::vector<uint32_t>{});

  // Assert
  ASSERT_EQ(groups_.size(), 1U);
  EXPECT_EQ((std::vector{s0, s1}), groups_[0]);
}

TEST_F(GroupSeriesByLabelNamesFixture, SkipUnusedSeriesIds) {
  // Arrange
  index_.find_or_emplace(LabelViewSet{{"lb", "z"}});
  const auto s0 = index_.find_or_emplace(LabelViewSet{{"la", "5"}, {"tag", "x"}});
  const auto s1 = index_.find_or_emplace(LabelViewSet{{"la", "5"}, {"tag", "y"}});
  const std::vector series_ids{s0, s1};
  const std::vector label_name_ids{name_id("la"), name_id("lb")};

  // Act
  group(series_ids, label_name_ids);

  // Assert
  ASSERT_EQ(groups_.size(), 1U);
  EXPECT_EQ(groups_[0], (std::vector{s0, s1}));
}

}  // namespace
