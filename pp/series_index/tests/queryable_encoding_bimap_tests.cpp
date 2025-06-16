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
using series_index::trie::CedarMatchesList;
using series_index::trie::CedarTrie;

class QueryableEncodingBimapFixture : public testing::Test {
 protected:
  using TrieIndex = series_index::TrieIndex<CedarTrie, CedarMatchesList>;
  using Lss = QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, BareBones::Vector, TrieIndex>;

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
    EXPECT_TRUE(series_ids->is_empty());
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

class QueryableEncodingBimapCopierFixture : public QueryableEncodingBimapFixture {
 protected:
  using Copier = QueryableEncodingBimapCopier<Lss>;
};

TEST_F(QueryableEncodingBimapCopierFixture, EmptyLss) {
  // Arrange
  Lss lss_copy;
  Copier copier(lss_, lss_copy);

  // Act
  copier.copy_added_series_and_build_indexes();

  // Assert
  EXPECT_EQ(0U, lss_copy.size());
}

TEST_F(QueryableEncodingBimapCopierFixture, NonEmptyLss) {
  // Arrange
  Lss lss_copy;
  Copier copier(lss_, lss_copy);

  const auto label_set = LabelViewSet{{"job", "cron"}, {"key", ""}, {"process", "php1"}};
  const auto label_set2 = LabelViewSet{{"job", "cron"}, {"key", ""}, {"process", "php"}};

  lss_.find_or_emplace(label_set);
  lss_.find_or_emplace(label_set2);

  // Act
  copier.copy_added_series_and_build_indexes();

  // Assert
  EXPECT_EQ(2U, lss_copy.size());
  EXPECT_EQ(0U, lss_copy.find(label_set));
  EXPECT_EQ(1U, lss_copy.find(label_set2));
  EXPECT_EQ(2U, lss_copy.reverse_index().names_count());
  EXPECT_THAT(lss_copy.ls_id_set(), testing::ElementsAre(1, 0));
  EXPECT_FALSE(lss_copy.ls_id_set().empty());
  EXPECT_TRUE(lss_copy.trie_index().names_trie().lookup("job"));
}

TEST_F(QueryableEncodingBimapCopierFixture, CopyOfCopy) {
  // Arrange
  Lss lss_copy;
  Lss lss_copy_of_copy;
  Copier copier(lss_, lss_copy);
  Copier copier2(lss_copy, lss_copy_of_copy);

  const auto label_set = LabelViewSet{{"job", "cron"}, {"key", ""}, {"process", "php"}};
  const auto label_set2 = LabelViewSet{{"job", "cron"}, {"key", ""}, {"process", "php1"}};
  const auto label_set3 = LabelViewSet{{"server", "localhost"}};

  lss_.find_or_emplace(label_set);
  lss_.find_or_emplace(label_set2);
  lss_.find_or_emplace(label_set3);

  // Act
  copier.copy_added_series_and_build_indexes();
  copier2.copy_added_series_and_build_indexes();
  lss_copy_of_copy.find_or_emplace(label_set3);

  // Assert
  EXPECT_EQ(1U, lss_copy_of_copy.size());
  EXPECT_FALSE(lss_copy_of_copy.find(label_set));
  EXPECT_FALSE(lss_copy_of_copy.find(label_set2));
  EXPECT_TRUE(lss_copy_of_copy.find(label_set3));
  EXPECT_EQ(1U, lss_copy_of_copy.reverse_index().names_count());
  EXPECT_THAT(lss_copy_of_copy.ls_id_set(), testing::ElementsAre(0));
  EXPECT_FALSE(lss_copy_of_copy.trie_index().names_trie().lookup("job"));
  EXPECT_TRUE(lss_copy_of_copy.trie_index().names_trie().lookup("server"));
}

}  // namespace
