#include <gmock/gmock-matchers.h>
#include <gtest/gtest.h>

#include "primitives/label_set.h"
#include "primitives/snug_composites.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using series_index::QueryableEncodingBimap;
using series_index::QueryableEncodingBimapCopier;
using series_index::SeriesReverseIndex;
using series_index::trie::CedarMatchesList;
using series_index::trie::CedarTrie;

template <class Src, class SortIndex, class Dst, class R>
using Copier = QueryableEncodingBimapCopier<Src, SortIndex, Dst, R>;

class QueryableEncodingBimapFixture : public testing::Test {
 protected:
  using Lss = QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, BareBones::Vector, CedarTrie>;

  Lss lss_;
};

TEST_F(QueryableEncodingBimapFixture, EmplaceLabelSet) {
  // Arrange

  // Act
  lss_.find_or_emplace(LabelViewSet{{"job", "cron"}});

  // Assert
  const auto name_id = lss_.trie_index().names_trie().lookup("job");
  EXPECT_TRUE(name_id);
  EXPECT_NE(nullptr, lss_.reverse_index().get(*name_id));

  const auto values_trie = lss_.trie_index().values_trie(*name_id);
  ASSERT_NE(nullptr, values_trie);
  EXPECT_TRUE(values_trie->lookup("cron"));
}

TEST_F(QueryableEncodingBimapFixture, EmplaceInvalidLabel) {
  // Arrange

  // Act
  LabelViewSet ls{{"key", "value"}};
  for (auto& label : ls) {
    label.second = "";
    break;
  }
  const auto ls_id = lss_.find_or_emplace(ls);

  // Assert
  const auto label = lss_[ls_id].begin();
  EXPECT_FALSE(lss_.trie_index().names_trie().lookup("key"));
  EXPECT_EQ(nullptr, lss_.reverse_index().get(label.name_id()));
  EXPECT_EQ(nullptr, lss_.trie_index().values_trie(label.name_id()));
}

TEST_F(QueryableEncodingBimapFixture, EmplaceLabelSetWithInvalidLabel) {
  // Arrange

  // Act
  LabelViewSet ls{{"job", "cron"}, {"key", "value"}, {"process", "php"}};
  for (auto& label : ls) {
    if (label.first == "key") {
      label.second = "";
      break;
    }
  }
  auto ls_id = lss_.find_or_emplace(ls);

  // Assert
  {
    auto name_id = lss_.trie_index().names_trie().lookup("job");
    EXPECT_TRUE(name_id);
    EXPECT_NE(nullptr, lss_.reverse_index().get(*name_id));

    auto values_trie = lss_.trie_index().values_trie(*name_id);
    ASSERT_NE(nullptr, values_trie);
    EXPECT_TRUE(values_trie->lookup("cron"));
  }

  {
    auto second_label = std::next(lss_[ls_id].begin());
    auto series_ids = lss_.reverse_index().get(second_label.name_id());

    EXPECT_FALSE(lss_.trie_index().names_trie().lookup("key"));
    ASSERT_NE(nullptr, series_ids);
    EXPECT_TRUE(series_ids->empty());
    EXPECT_EQ(nullptr, lss_.trie_index().values_trie(second_label.name_id()));
  }

  {
    auto name_id = lss_.trie_index().names_trie().lookup("process");
    EXPECT_TRUE(name_id);
    EXPECT_NE(nullptr, lss_.reverse_index().get(*name_id));

    auto values_trie = lss_.trie_index().values_trie(*name_id);
    ASSERT_NE(nullptr, values_trie);
    EXPECT_TRUE(values_trie->lookup("php"));
  }
}

TEST_F(QueryableEncodingBimapFixture, EmplaceDuplicatedLabelSet) {
  // Arrange
  const auto label_set = LabelViewSet{{"job", "cron"}, {"key", ""}, {"process", "php"}};
  const auto label_set2 = LabelViewSet{{"job", "cron"}, {"key", ""}, {"process", "php1"}};

  // Act
  const auto ls_id1 = lss_.find_or_emplace(label_set);
  const auto existing_ls_id = lss_.find_or_emplace(label_set);
  const auto ls_id2 = lss_.find_or_emplace(label_set2);

  // Assert
  EXPECT_EQ(ls_id1, existing_ls_id);
  EXPECT_NE(ls_id1, ls_id2);
}

class QueryableEncodingBimapCopierFixture : public QueryableEncodingBimapFixture {};

TEST_F(QueryableEncodingBimapCopierFixture, EmptyLss) {
  // Arrange
  Lss lss_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy);

  // Act
  copier.copy_added_series_and_build_indexes();

  // Assert
  EXPECT_EQ(0U, lss_copy.size());
}

TEST_F(QueryableEncodingBimapCopierFixture, NonEmptyLss) {
  // Arrange
  const auto label_set = LabelViewSet{{"job", "cron"}, {"key", ""}, {"process", "php1"}};
  const auto label_set2 = LabelViewSet{{"job", "cron"}, {"key", ""}, {"process", "php"}};

  lss_.find_or_emplace(label_set);
  lss_.find_or_emplace(label_set2);

  lss_.build_deferred_indexes();

  Lss lss_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy);

  // Act
  copier.copy_added_series_and_build_indexes();

  // Assert
  EXPECT_EQ(2U, lss_copy.size());

  EXPECT_TRUE(lss_copy.find(label_set).has_value());
  EXPECT_TRUE(lss_copy.find(label_set2).has_value());

  EXPECT_EQ(2U, lss_copy.reverse_index().names_count());

  EXPECT_EQ(2U, lss_copy.ls_id_set().size());
  EXPECT_FALSE(lss_copy.ls_id_set().empty());

  EXPECT_TRUE(lss_copy.trie_index().names_trie().lookup("job"));
}

TEST_F(QueryableEncodingBimapCopierFixture, NonEmptyLssKeepOrder) {
  // Arrange
  const auto label_set1 = LabelViewSet{{"job", "cron"}, {"key", "1"}, {"process", "php"}};
  const auto label_set2 = LabelViewSet{{"job", "cron"}, {"key", "2"}, {"process", "php"}};
  const auto label_set3 = LabelViewSet{{"job", "cron"}, {"key", "3"}, {"process", "php"}};
  const auto label_set4 = LabelViewSet{{"job", "cron"}, {"key", "4"}, {"process", "php"}};
  const auto label_set5 = LabelViewSet{{"job", "cron"}, {"key", "5"}, {"process", "php"}};

  lss_.find_or_emplace(label_set4);
  lss_.find_or_emplace(label_set1);
  lss_.find_or_emplace(label_set3);
  lss_.find_or_emplace(label_set5);
  lss_.find_or_emplace(label_set2);

  lss_.build_deferred_indexes();

  Lss lss_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy);

  // Act
  copier.copy_added_series_and_build_indexes();

  // Assert
  EXPECT_TRUE(std::ranges::is_sorted(lss_copy.ls_id_set(),
                                     [&](const auto idl, const auto idr) { return std::ranges::lexicographical_compare(lss_copy[idl], lss_copy[idr]); }));
}

TEST_F(QueryableEncodingBimapCopierFixture, CopyOfCopy) {
  // Arrange
  Lss lss_copy;
  Lss lss_copy_of_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy);
  Copier copier2(lss_copy, lss_copy.sorting_index(), lss_copy.added_series(), lss_copy_of_copy);

  const auto label_set = LabelViewSet{{"job", "cron"}, {"key", ""}, {"process", "php"}};
  const auto label_set2 = LabelViewSet{{"job", "cron"}, {"key", ""}, {"process", "php1"}};
  const auto label_set3 = LabelViewSet{{"server", "localhost"}};

  lss_.find_or_emplace(label_set);
  lss_.find_or_emplace(label_set2);
  lss_.find_or_emplace(label_set3);

  lss_.build_deferred_indexes();

  // Act
  copier.copy_added_series_and_build_indexes();
  lss_copy.build_deferred_indexes();
  copier2.copy_added_series_and_build_indexes();
  lss_copy_of_copy.find_or_emplace(label_set3);

  // Assert
  EXPECT_EQ(1U, lss_copy_of_copy.size());

  EXPECT_FALSE(lss_copy_of_copy.find(label_set));
  EXPECT_FALSE(lss_copy_of_copy.find(label_set2));
  EXPECT_TRUE(lss_copy_of_copy.find(label_set3));

  EXPECT_EQ(1U, lss_copy_of_copy.reverse_index().names_count());

  EXPECT_EQ(1U, lss_copy_of_copy.ls_id_set().size());

  EXPECT_FALSE(lss_copy_of_copy.trie_index().names_trie().lookup("job"));
  EXPECT_TRUE(lss_copy_of_copy.trie_index().names_trie().lookup("server"));
}

}  // namespace
