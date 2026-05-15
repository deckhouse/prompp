#include <gmock/gmock-matchers.h>
#include <gtest/gtest.h>
#include <tuple>
#include <vector>

#include "primitives/label_set.h"
#include "primitives/snug_composites.h"
#include "series_index/queryable_encoding_bimap.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using series_index::QueryableEncodingBimap;
using series_index::QueryableEncodingBimapCopier;

template <class DecodingTable, class SortingIndex, class SeriesIds, class QueryableEncodingBimap, class LsIdVector>
using Copier = QueryableEncodingBimapCopier<DecodingTable, SortingIndex, SeriesIds, QueryableEncodingBimap, LsIdVector>;

class BimapFixture : public testing::Test {
 protected:
  using Lss = QueryableEncodingBimap<BareBones::Vector>;

  Lss lss_;

  static void load_lss_with_single_series(const LabelViewSet& label_set, Lss& loaded, uint32_t& loaded_id) {
    Lss source;
    source.find_or_emplace(label_set);

    std::stringstream stream;
    stream << source;

    stream >> loaded;

    const auto from_find = loaded.find(label_set);
    assert(from_find.has_value());
    loaded_id = *from_find;
  }

  static LabelViewSet LabelSetWithInvalidMiddle() {
    LabelViewSet ls{{"job", "cron"}, {"key", "value"}, {"process", "php"}};
    for (auto& label : ls) {
      if (label.first == "key") {
        label.second = "";
        break;
      }
    }
    return ls;
  }
};

TEST_F(BimapFixture, EmptyCompositeHasZeroSize) {
  // Arrange
  typename Lss::Base::value_type empty;

  // Act

  // Assert
  EXPECT_EQ(0U, empty.size());
}

TEST_F(BimapFixture, MarkActiveAffectsOnlyAddedSeries) {
  // Arrange

  // Act
  lss_.mark_active(1234);

  // Assert
  ASSERT_EQ(lss_.size(), 0U);

  EXPECT_TRUE(lss_.added_series()[1234]);
  EXPECT_EQ(lss_.added_series().popcount(), 1U);
}

TEST_F(BimapFixture, EmplaceLabelSet) {
  // Arrange

  // Act
  lss_.find_or_emplace(LabelViewSet{{"job", "cron"}});

  const auto name_id = lss_.trie_index().names_trie().lookup("job");
  const auto values_trie = lss_.trie_index().values_trie(*name_id);

  // Assert
  EXPECT_TRUE(name_id);
  EXPECT_NE(nullptr, lss_.reverse_index().get(*name_id));

  ASSERT_NE(nullptr, values_trie);
  EXPECT_TRUE(values_trie->lookup("cron"));
}

TEST_F(BimapFixture, EmplaceInvalidLabelSkipsTrie) {
  // Arrange
  LabelViewSet ls{{"key", "value"}};
  for (auto& label : ls) {
    label.second = "";
    break;
  }

  // Act
  const auto ls_id = lss_.find_or_emplace(ls);
  const auto label = lss_[ls_id].begin();

  // Assert
  EXPECT_FALSE(lss_.trie_index().names_trie().lookup("key"));
  EXPECT_EQ(nullptr, lss_.reverse_index().get(label.name_id()));
  EXPECT_EQ(nullptr, lss_.trie_index().values_trie(label.name_id()));
}

TEST_F(BimapFixture, EmplaceInvalidMiddleIndexesFirstValidLabel) {
  // Arrange
  const auto ls = LabelSetWithInvalidMiddle();

  // Act
  lss_.find_or_emplace(ls);
  const auto name_id = lss_.trie_index().names_trie().lookup("job");

  // Assert
  ASSERT_TRUE(name_id);
  EXPECT_NE(nullptr, lss_.reverse_index().get(*name_id));
  const auto values_trie = lss_.trie_index().values_trie(*name_id);
  ASSERT_NE(nullptr, values_trie);
  EXPECT_TRUE(values_trie->lookup("cron"));
}

TEST_F(BimapFixture, EmplaceInvalidMiddleSkipsInvalidName) {
  // Arrange
  const auto ls = LabelSetWithInvalidMiddle();
  const auto ls_id = lss_.find_or_emplace(ls);

  // Act
  const auto second_label = std::next(lss_[ls_id].begin());
  const auto series_ids = lss_.reverse_index().get(second_label.name_id());

  // Assert
  EXPECT_FALSE(lss_.trie_index().names_trie().lookup("key"));

  ASSERT_NE(nullptr, series_ids);
  EXPECT_TRUE(series_ids->empty());
  EXPECT_EQ(nullptr, lss_.trie_index().values_trie(second_label.name_id()));
}

TEST_F(BimapFixture, EmplaceInvalidMiddleIndexesLastValidLabel) {
  // Arrange
  const auto ls = LabelSetWithInvalidMiddle();

  // Act
  lss_.find_or_emplace(ls);
  const auto name_id = lss_.trie_index().names_trie().lookup("process");
  const auto values_trie = lss_.trie_index().values_trie(*name_id);

  // Assert

  ASSERT_TRUE(name_id);
  EXPECT_NE(nullptr, lss_.reverse_index().get(*name_id));

  ASSERT_NE(nullptr, values_trie);
  EXPECT_TRUE(values_trie->lookup("php"));
}

TEST_F(BimapFixture, EmplaceDuplicateLabelSetReturnsSameId) {
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

TEST_F(BimapFixture, LoadRoundTripRestoresSeriesIds) {
  // Arrange
  const auto label_set1 = LabelViewSet{{"job", "cron"}, {"key", ""}, {"process", "php"}};
  const auto label_set2 = LabelViewSet{{"job", "cron"}, {"key", ""}, {"process", "php1"}};

  const auto ls_id1 = lss_.find_or_emplace(label_set1);
  const auto ls_id2 = lss_.find_or_emplace(label_set2);

  std::stringstream stream;
  stream << lss_;

  Lss lss2;

  // Act
  stream >> lss2;

  // Assert
  EXPECT_EQ(ls_id1, lss2.find(label_set1));
  EXPECT_EQ(ls_id2, lss2.find(label_set2));
}

TEST_F(BimapFixture, InsertedSeriesUpdatesSeriesCount) {
  // Arrange
  lss_.find_or_emplace(LabelViewSet{{"job", "cron"}});
  lss_.find_or_emplace(LabelViewSet{{"job", "worker"}});

  // Act

  // Assert
  EXPECT_EQ(2U, lss_.items_count());
}

TEST_F(BimapFixture, InsertedSeriesUpdatesMaxItemIndex) {
  // Arrange
  lss_.find_or_emplace(LabelViewSet{{"job", "cron"}});
  lss_.find_or_emplace(LabelViewSet{{"job", "worker"}});

  // Act

  // Assert
  EXPECT_EQ(2U, lss_.next_item_index());
}

class BimapCopierFixtureBase : public BimapFixture {
 protected:
  const LabelViewSet ls0_{{"job", "a"}};
  const LabelViewSet ls1_{{"job", "b"}};
  const LabelViewSet ls2_{{"job", "c"}};
  const LabelViewSet ls3_{{"job", "d"}};
  const LabelViewSet ls4_{{"job", "e"}};

  BareBones::Vector<uint32_t> dst_src_ids_mapping_;
};

class BimapStatesFixture : public BimapCopierFixtureBase {
 protected:
  void SetUp() override {
    Lss initial_lss;

    initial_lss.find_or_emplace(ls0_);
    initial_lss.find_or_emplace(ls1_);
    initial_lss.find_or_emplace(ls2_);
    initial_lss.find_or_emplace(ls3_);
    initial_lss.find_or_emplace(ls4_);
    initial_lss.build_deferred_indexes();

    BareBones::Vector<uint32_t> dst_src_ids_mapping;

    Copier copier(initial_lss, initial_lss.sorting_index(), initial_lss.added_series(), lss_, dst_src_ids_mapping);
    copier.copy_added_series_and_build_indexes();

    // Keep ids >= shrink boundary alive and preserve one pre-boundary series.
    [[maybe_unused]] const auto touched_ls1 = lss_.find_or_emplace(ls1_);
    [[maybe_unused]] const auto touched_ls3 = lss_.find_or_emplace(ls3_);
    [[maybe_unused]] const auto touched_ls4 = lss_.find_or_emplace(ls4_);
    lss_.build_deferred_indexes();
  }
};

TEST_F(BimapStatesFixture, LssInitialState) {
  // Arrange

  // Act

  // Assert
  EXPECT_EQ(5U, lss_.items_count());
  EXPECT_EQ(5U, lss_.next_item_index());

  EXPECT_TRUE(std::ranges::equal(lss_.added_series(), std::vector<uint32_t>{1, 3, 4}));
}

TEST_F(BimapStatesFixture, FixedStateResolveBehavesLikeNormal) {
  // Arrange
  auto before_shrink = lss_[2];

  // Act
  constexpr uint32_t shrink_boundary = 3U;
  lss_.set_pending_shrink_boundary(shrink_boundary);
  const auto after_shrink = lss_[2];

  // Assert
  EXPECT_EQ(before_shrink, after_shrink);
}

TEST_F(BimapStatesFixture, FixedStateFindHidesPreBoundarySeries) {
  // Arrange
  const auto from_find_before_fixed = lss_.find(ls2_);

  // Act
  constexpr uint32_t shrink_boundary = 3U;
  lss_.set_pending_shrink_boundary(shrink_boundary);
  const auto from_find = lss_.find(ls2_);

  // Assert
  ASSERT_TRUE(from_find_before_fixed.has_value());
  EXPECT_FALSE(from_find.has_value());
}

TEST_F(BimapStatesFixture, NormalStateFillsNonaddedSeries) {
  // Arrange

  // Act
  const auto ls_id = lss_.find_or_emplace(ls0_);

  // Assert
  EXPECT_EQ(ls_id, 0U);
  EXPECT_EQ(ls0_, lss_[0]);
  EXPECT_TRUE(lss_.added_series()[0]);
}

TEST_F(BimapStatesFixture, FixedStateAppendPastBoundary) {
  // Arrange

  // Act
  constexpr uint32_t shrink_boundary = 3U;
  lss_.set_pending_shrink_boundary(shrink_boundary);
  const auto ls_id = lss_.find_or_emplace(ls0_);

  // Assert
  EXPECT_EQ(ls_id, 5);
  EXPECT_EQ(lss_[0], lss_[5]);

  EXPECT_TRUE(lss_.added_series()[5]);
  EXPECT_FALSE(lss_.added_series()[0]);
}

class BimapCopierFixture : public BimapCopierFixtureBase {
 protected:
  void SetUp() override {
    lss_.find_or_emplace(ls0_);
    lss_.find_or_emplace(ls1_);
    lss_.find_or_emplace(ls2_);
    lss_.find_or_emplace(ls3_);
    lss_.find_or_emplace(ls4_);
    lss_.build_deferred_indexes();
  }
};

TEST_F(BimapCopierFixtureBase, CopyFromEmptySourceLeavesEmpty) {
  // Arrange
  Lss lss_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);

  // Act
  copier.copy_added_series_and_build_indexes();

  // Assert
  EXPECT_EQ(0U, lss_copy.items_count());
  EXPECT_EQ(0U, dst_src_ids_mapping_.size());
}

TEST_F(BimapCopierFixture, CopyKeepsSeriesCountAndFinds) {
  // Arrange
  const BareBones::Vector ids_for_copy{0U, 1U};
  Lss lss_copy;

  // Act
  dst_src_ids_mapping_.clear();
  Copier copier(lss_, lss_.sorting_index(), ids_for_copy, lss_copy, dst_src_ids_mapping_);
  copier.copy_added_series_and_build_indexes();

  // Assert
  EXPECT_EQ(2U, lss_copy.items_count());
  EXPECT_TRUE(lss_copy.find(ls0_).has_value());
  EXPECT_TRUE(lss_copy.find(ls1_).has_value());
}

TEST_F(BimapCopierFixture, CopyBuildsTrieAndPostingIndexes) {
  // Arrange
  const BareBones::Vector ids_for_copy{0U, 1U};
  Lss lss_copy;

  // Act
  dst_src_ids_mapping_.clear();
  Copier copier(lss_, lss_.sorting_index(), ids_for_copy, lss_copy, dst_src_ids_mapping_);
  copier.copy_added_series_and_build_indexes();

  // Assert
  EXPECT_EQ(1U, lss_copy.reverse_index().names_count());
  EXPECT_EQ(2U, lss_copy.ls_id_set().size());

  EXPECT_FALSE(lss_copy.ls_id_set().empty());
  EXPECT_TRUE(lss_copy.trie_index().names_trie().lookup("job"));
}

TEST_F(BimapCopierFixture, CopyPreservesSourceIdMapping) {
  // Arrange
  Lss lss_copy;
  const BareBones::Vector ids_for_copy{0U, 2U, 4U};
  Copier copier(lss_, lss_.sorting_index(), ids_for_copy, lss_copy, dst_src_ids_mapping_);

  // Act
  copier.copy_added_series_and_build_indexes();

  // Assert
  ASSERT_EQ(3U, lss_copy.items_count());

  EXPECT_EQ(ls0_, lss_copy[0]);
  EXPECT_EQ(ls2_, lss_copy[1]);
  EXPECT_EQ(ls4_, lss_copy[2]);

  EXPECT_EQ(ids_for_copy, dst_src_ids_mapping_);
}

TEST_F(BimapCopierFixture, CopyPreservesLexicographicOrder) {
  // Arrange
  Lss lss_copy;
  const BareBones::Vector ids_for_copy{0U, 2U, 4U};
  Copier copier(lss_, lss_.sorting_index(), ids_for_copy, lss_copy, dst_src_ids_mapping_);

  // Act
  copier.copy_added_series_and_build_indexes();

  // Assert
  EXPECT_TRUE(std::ranges::is_sorted(lss_copy.ls_id_set(),
                                     [&](const auto idl, const auto idr) { return std::ranges::lexicographical_compare(lss_copy[idl], lss_copy[idr]); }));
  EXPECT_EQ(ids_for_copy, dst_src_ids_mapping_);
}

TEST_F(BimapCopierFixture, CopyOfCopyWithNoNewAddedIsEmpty) {
  // Arrange
  Lss lss_copy;
  Lss lss_copy_of_copy;

  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);
  Copier copier2(lss_copy, lss_copy.sorting_index(), lss_copy.added_series(), lss_copy_of_copy, dst_src_ids_mapping_);

  // Act
  copier.copy_added_series_and_build_indexes();
  lss_copy.build_deferred_indexes();
  copier2.copy_added_series_and_build_indexes();

  // Assert
  EXPECT_EQ(0U, lss_copy_of_copy.items_count());
  EXPECT_TRUE(dst_src_ids_mapping_.empty());

  EXPECT_FALSE(lss_copy_of_copy.find(ls0_).has_value());
  EXPECT_FALSE(lss_copy_of_copy.find(ls1_).has_value());
  EXPECT_FALSE(lss_copy_of_copy.find(ls2_).has_value());
  EXPECT_FALSE(lss_copy_of_copy.find(ls3_).has_value());
  EXPECT_FALSE(lss_copy_of_copy.find(ls4_).has_value());
}

TEST_F(BimapCopierFixture, CopyOfCopyInsertGetsFirstId) {
  // Arrange
  Lss lss_copy;
  Lss lss_copy_of_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);
  Copier copier2(lss_copy, lss_copy.sorting_index(), lss_copy.added_series(), lss_copy_of_copy, dst_src_ids_mapping_);

  const auto label_set3 = LabelViewSet{{"server", "localhost"}};

  copier.copy_added_series_and_build_indexes();
  lss_copy.build_deferred_indexes();
  copier2.copy_added_series_and_build_indexes();

  // Act
  const auto ls_id = lss_copy_of_copy.find_or_emplace(label_set3);

  // Assert
  EXPECT_EQ(0U, ls_id);
  EXPECT_TRUE(lss_copy_of_copy.find(label_set3).has_value());
  EXPECT_EQ(1U, lss_copy_of_copy.items_count());
}

TEST_F(BimapCopierFixture, CopyOfCopyInsertBuildsIndexesForNewSeries) {
  // Arrange
  Lss lss_copy;
  Lss lss_copy_of_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);
  Copier copier2(lss_copy, lss_copy.sorting_index(), lss_copy.added_series(), lss_copy_of_copy, dst_src_ids_mapping_);

  const auto label_set3 = LabelViewSet{{"server", "localhost"}};

  copier.copy_added_series_and_build_indexes();
  lss_copy.build_deferred_indexes();
  copier2.copy_added_series_and_build_indexes();

  // Act
  std::ignore = lss_copy_of_copy.find_or_emplace(label_set3);

  // Assert
  EXPECT_EQ(1U, lss_copy_of_copy.reverse_index().names_count());
  EXPECT_EQ(1U, lss_copy_of_copy.ls_id_set().size());
  EXPECT_FALSE(lss_copy_of_copy.trie_index().names_trie().lookup("job"));
  EXPECT_TRUE(lss_copy_of_copy.trie_index().names_trie().lookup("server"));
}

TEST_F(BimapCopierFixture, FinalizeShrinkKeepsTrieAfterTwoSeries) {
  // Arrange
  Lss lss_copy;
  const BareBones::Vector<uint32_t> ids_for_snapshot{0U, 1U};
  dst_src_ids_mapping_.clear();
  Copier copier(lss_, lss_.sorting_index(), ids_for_snapshot, lss_copy, dst_src_ids_mapping_);
  copier.copy_added_series_and_build_indexes();

  constexpr uint32_t shrink_boundary = 2U;

  // Act
  lss_.set_pending_shrink_boundary(shrink_boundary);
  lss_.finalize_copy_and_shrink(lss_copy, dst_src_ids_mapping_);

  // Assert
  EXPECT_TRUE(lss_.trie_index().names_trie().lookup("job"));
  ASSERT_NE(nullptr, lss_.trie_index().values_trie(*lss_.trie_index().names_trie().lookup("job")));
  EXPECT_EQ(ls0_, lss_[0]);
  EXPECT_EQ(ls1_, lss_[1]);
}

class BimapShrinkFixture : public BimapCopierFixtureBase {
 protected:
  void SetUp() override {
    lss_.find_or_emplace(ls0_);
    lss_.find_or_emplace(ls1_);
    lss_.find_or_emplace(ls2_);
    lss_.build_deferred_indexes();
  }
};

TEST_F(BimapShrinkFixture, FinalizeShrinkMapsSeriesInOrderAndKeepsCounts) {
  // Arrange
  const uint32_t shrink_boundary = lss_.next_item_index();
  Lss lss_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);
  copier.copy_added_series_and_build_indexes();
  constexpr uint32_t kLogicalSeriesCount = 3U;

  // Act
  lss_.set_pending_shrink_boundary(shrink_boundary);
  lss_.finalize_copy_and_shrink(lss_copy, dst_src_ids_mapping_);

  // Assert
  EXPECT_EQ(ls0_, lss_[0]);
  EXPECT_EQ(ls1_, lss_[1]);
  EXPECT_EQ(ls2_, lss_[2]);
  EXPECT_EQ(kLogicalSeriesCount, lss_.items_count());
  EXPECT_EQ(kLogicalSeriesCount, lss_.next_item_index());
}

class BimapFixedStateFixture : public BimapCopierFixtureBase {
 protected:
  static constexpr uint32_t kPendingShrinkBoundary = 2;

  void SetUp() override {
    lss_.find_or_emplace(ls0_);
    lss_.find_or_emplace(ls1_);
    lss_.set_pending_shrink_boundary(kPendingShrinkBoundary);
    lss_.find_or_emplace(ls2_);
  }
};

TEST_F(BimapFixedStateFixture, FixedStateFindMatchesFindOrEmplace) {
  // Arrange

  // Act
  const auto from_find = lss_.find(ls2_);
  const auto from_find_or_emplace = lss_.find_or_emplace(ls2_);

  // Assert
  ASSERT_TRUE(from_find.has_value());
  EXPECT_EQ(kPendingShrinkBoundary, *from_find);
  EXPECT_EQ(from_find_or_emplace, *from_find);
  EXPECT_TRUE(std::ranges::equal(ls2_, lss_[*from_find]));
}

TEST_F(BimapFixedStateFixture, FixedStateSeriesCountMatchesThree) {
  // Arrange

  // Act

  // Assert
  EXPECT_EQ(3U, lss_.items_count());
  EXPECT_EQ(3U, lss_.next_item_index());
}

TEST_F(BimapFixedStateFixture, FixedStateBracketExposesVisibleSeries) {
  // Arrange

  // Act
  const auto id = lss_.find(ls2_);

  // Assert
  ASSERT_TRUE(id.has_value());
  EXPECT_EQ(kPendingShrinkBoundary, *id);
  EXPECT_TRUE(std::ranges::equal(ls0_, lss_[0]));
  EXPECT_TRUE(std::ranges::equal(ls1_, lss_[1]));
  EXPECT_TRUE(std::ranges::equal(ls2_, lss_[2]));
  EXPECT_TRUE(std::ranges::equal(ls2_, lss_[*id]));
}

class BimapFixedPendingFixture : public BimapCopierFixtureBase {
 protected:
  static constexpr uint32_t kPendingShrinkBoundary = 2;

  void SetUp() override {
    lss_.find_or_emplace(ls0_);
    lss_.find_or_emplace(ls1_);
    lss_.set_pending_shrink_boundary(kPendingShrinkBoundary);
  }
};

TEST_F(BimapFixedPendingFixture, FixedStateInsertTailGetsBoundaryOrAbove) {
  // Arrange

  // Act
  const auto id = lss_.find_or_emplace(ls2_);

  // Assert
  EXPECT_EQ(kPendingShrinkBoundary, id);
  const auto from_find = lss_.find(ls2_);
  ASSERT_TRUE(from_find.has_value());
  EXPECT_EQ(id, *from_find);
  EXPECT_TRUE(std::ranges::equal(ls2_, lss_[id]));
}

TEST_F(BimapFixedPendingFixture, FixedStateTwoSeriesCountMatchesStorage) {
  // Arrange

  // Act

  // Assert
  EXPECT_EQ(2U, lss_.items_count());
  EXPECT_EQ(2U, lss_.next_item_index());
}

class BimapFiveSeriesFixture : public BimapCopierFixture {
 protected:
  const LabelViewSet ls6_{{"job", "f"}};
  Lss lss_copy_;

  void SetUp() override { BimapCopierFixture::SetUp(); }

  template <class SeriesIds>
  void FinalizeShrink(const SeriesIds& ids_for_copy, uint32_t shrink_boundary) {
    dst_src_ids_mapping_.clear();
    Copier copier(lss_, lss_.sorting_index(), ids_for_copy, lss_copy_, dst_src_ids_mapping_);
    copier.copy_added_series_and_build_indexes();

    lss_.set_pending_shrink_boundary(shrink_boundary);
    lss_.finalize_copy_and_shrink(lss_copy_, dst_src_ids_mapping_);
  }
};

TEST_F(BimapFiveSeriesFixture, ShrinkFindOrEmplaceAddsAtBoundary) {
  // Arrange
  const uint32_t shrink_boundary = lss_.next_item_index();
  FinalizeShrink(lss_.added_series(), shrink_boundary);

  // Act
  const auto new_id = lss_.find_or_emplace(ls6_);
  const auto from_find = lss_.find(ls6_);

  // Assert
  EXPECT_EQ(shrink_boundary, new_id);
  ASSERT_TRUE(from_find.has_value());
  EXPECT_EQ(new_id, *from_find);
  EXPECT_EQ(ls6_, lss_[new_id]);
}

TEST_F(BimapFiveSeriesFixture, ShrinkMiddleCopyKeepsSeriesLayout) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;
  FinalizeShrink(lss_.added_series(), shrink_boundary);

  // Act
  const auto from_find_ls1 = lss_.find(ls1_);

  // Assert
  EXPECT_EQ(ls0_, lss_[0]);
  EXPECT_EQ(ls1_, lss_[1]);
  EXPECT_EQ(ls2_, lss_[2]);
  EXPECT_EQ(ls3_, lss_[3]);
  EXPECT_EQ(ls4_, lss_[4]);
  ASSERT_TRUE(from_find_ls1.has_value());
  EXPECT_EQ(1U, *from_find_ls1);
}

TEST_F(BimapFiveSeriesFixture, ShrinkMiddleCopyLsIdSetHasLogicalIds) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;
  const auto ids_for_copy = lss_.added_series();

  // Act
  FinalizeShrink(ids_for_copy, shrink_boundary);

  // Assert
  const auto& ls_ids = lss_.ls_id_set();
  using LsIdProxy = typename Lss::LsIdSet::value_type;
  EXPECT_EQ(5U, ls_ids.size());
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{0U}));
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{1U}));
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{2U}));
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{3U}));
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{4U}));
}

TEST_F(BimapFiveSeriesFixture, ShrinkMiddleCopyFindOrEmplaceIdempotent) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;
  FinalizeShrink(lss_.added_series(), shrink_boundary);

  // Act
  const auto existing_id = lss_.find_or_emplace(ls1_);
  const auto from_find_after = lss_.find(ls1_);

  // Assert
  EXPECT_EQ(1U, existing_id);
  ASSERT_TRUE(from_find_after.has_value());
  EXPECT_EQ(existing_id, *from_find_after);
  EXPECT_EQ(ls1_, lss_[existing_id]);
}

TEST_F(BimapFiveSeriesFixture, ShrunkReinsertMarksAddedAndLsIdSet) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;
  constexpr uint32_t expected_existing_id = 1U;
  FinalizeShrink(lss_.added_series(), shrink_boundary);

  // Act
  const auto new_id = lss_.find_or_emplace(ls1_);

  // Assert
  using LsIdProxy = typename Lss::LsIdSet::value_type;

  ASSERT_EQ(expected_existing_id, new_id);
  ASSERT_LT(new_id, lss_.added_series().size());
  EXPECT_TRUE(lss_.added_series()[new_id]);
  EXPECT_TRUE(lss_.ls_id_set().contains(LsIdProxy{new_id}));
}

TEST_F(BimapFiveSeriesFixture, ShrinkFullCopyKeepsSeriesLayout) {
  // Arrange
  const uint32_t shrink_boundary = lss_.next_item_index();
  FinalizeShrink(lss_.added_series(), shrink_boundary);

  // Act
  const auto from_find = lss_.find(ls1_);

  // Assert
  EXPECT_EQ(ls0_, lss_[0]);
  EXPECT_EQ(ls1_, lss_[1]);
  EXPECT_EQ(ls2_, lss_[2]);
  EXPECT_EQ(ls3_, lss_[3]);
  EXPECT_EQ(ls4_, lss_[4]);
  ASSERT_TRUE(from_find.has_value());
  EXPECT_EQ(1U, *from_find);
}

TEST_F(BimapFiveSeriesFixture, ShrinkFullCopyLsIdSetHasLogicalIds) {
  // Arrange
  const uint32_t shrink_boundary = lss_.next_item_index();
  const auto ids_for_copy = lss_.added_series();

  // Act
  FinalizeShrink(ids_for_copy, shrink_boundary);

  // Assert
  const auto& ls_ids = lss_.ls_id_set();
  using LsIdProxy = typename Lss::LsIdSet::value_type;
  EXPECT_EQ(5U, ls_ids.size());
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{0U}));
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{1U}));
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{2U}));
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{3U}));
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{4U}));
}

TEST_F(BimapFiveSeriesFixture, ShrinkFullCopyFindOrEmplaceIdempotent) {
  // Arrange
  const uint32_t shrink_boundary = lss_.next_item_index();
  FinalizeShrink(lss_.added_series(), shrink_boundary);

  // Act
  const auto new_id = lss_.find_or_emplace(ls1_);
  const auto from_find_after = lss_.find(ls1_);

  // Assert
  EXPECT_EQ(1U, new_id);
  ASSERT_TRUE(from_find_after.has_value());
  EXPECT_EQ(new_id, *from_find_after);
  EXPECT_EQ(ls1_, lss_[new_id]);
}

TEST_F(BimapFiveSeriesFixture, ShrunkStateFindResolvesTailIds) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;
  FinalizeShrink(lss_.added_series(), shrink_boundary);

  // Act
  const auto ls3_id = lss_.find(ls3_);
  const auto ls4_id = lss_.find(ls4_);

  // Assert
  ASSERT_TRUE(ls3_id.has_value());
  ASSERT_TRUE(ls4_id.has_value());
  EXPECT_EQ(shrink_boundary, *ls3_id);
  EXPECT_EQ(shrink_boundary + 1, *ls4_id);
  EXPECT_EQ(ls3_, lss_[*ls3_id]);
  EXPECT_EQ(ls4_, lss_[*ls4_id]);
}

class BimapPartialAddedFixture : public BimapCopierFixtureBase {
 protected:
  Lss seeded_lss_;
  Lss lss_copy_;

  void SetUp() override {
    seeded_lss_.find_or_emplace(ls0_);
    seeded_lss_.find_or_emplace(ls1_);
    seeded_lss_.find_or_emplace(ls2_);
    seeded_lss_.find_or_emplace(ls3_);
    seeded_lss_.find_or_emplace(ls4_);
    seeded_lss_.build_deferred_indexes();

    dst_src_ids_mapping_.clear();
    Copier copier(seeded_lss_, seeded_lss_.sorting_index(), seeded_lss_.added_series(), lss_, dst_src_ids_mapping_);
    copier.copy_added_series_and_build_indexes();

    // Keep ids >= shrink boundary alive and preserve one pre-boundary series.
    [[maybe_unused]] const auto touched_ls1 = lss_.find_or_emplace(ls1_);
    [[maybe_unused]] const auto touched_ls3 = lss_.find_or_emplace(ls3_);
    [[maybe_unused]] const auto touched_ls4 = lss_.find_or_emplace(ls4_);
    lss_.build_deferred_indexes();
  }

  template <class SeriesIds>
  void FinalizeShrink(const SeriesIds& ids_for_copy, uint32_t shrink_boundary) {
    dst_src_ids_mapping_.clear();
    Copier copier(lss_, lss_.sorting_index(), ids_for_copy, lss_copy_, dst_src_ids_mapping_);
    copier.copy_added_series_and_build_indexes();

    lss_.set_pending_shrink_boundary(shrink_boundary);
    lss_.finalize_copy_and_shrink(lss_copy_, dst_src_ids_mapping_);
  }

  [[nodiscard]] std::vector<uint32_t> ls_logical_ids_in_set_order() const {
    std::vector<uint32_t> ids;
    ids.reserve(lss_.ls_id_set().size());
    for (const auto id : lss_.ls_id_set()) {
      ids.emplace_back(static_cast<uint32_t>(id));
    }
    return ids;
  }
};

TEST_F(BimapPartialAddedFixture, FixedStatePrunesHiddenPreBoundaryFromLsIdSet) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;
  LabelViewSet ls6_{{"job", "f"}};

  lss_.set_pending_shrink_boundary(shrink_boundary);

  // Act
  const auto new_id = lss_.find_or_emplace(ls6_);

  // Assert
  using LsIdProxy = typename Lss::LsIdSet::value_type;
  const auto& ls_ids = lss_.ls_id_set();
  EXPECT_FALSE(ls_ids.contains(LsIdProxy{0U}));
  EXPECT_FALSE(ls_ids.contains(LsIdProxy{2U}));
  EXPECT_GE(ls_ids.size(), 4U);
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{new_id}));
}

TEST_F(BimapPartialAddedFixture, FixedStateLsIdSetOrderStaysSortedByCurrentResolve) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;
  lss_.set_pending_shrink_boundary(shrink_boundary);

  // Act
  const std::vector<uint32_t> ids_in_tree_order = ls_logical_ids_in_set_order();

  // Assert
  EXPECT_TRUE(
      std::ranges::is_sorted(ids_in_tree_order, [this](uint32_t lhs, uint32_t rhs) { return std::ranges::lexicographical_compare(lss_[lhs], lss_[rhs]); }));
}

TEST_F(BimapPartialAddedFixture, FixedStateSortingIndexSortKeepsCurrentResolveOrder) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;
  lss_.set_pending_shrink_boundary(shrink_boundary);
  std::vector<uint32_t> ids = ls_logical_ids_in_set_order();

  // Act
  std::ranges::reverse(ids);
  lss_.sorting_index().sort(ids);

  // Assert
  EXPECT_TRUE(std::ranges::is_sorted(ids, [this](uint32_t lhs, uint32_t rhs) { return std::ranges::lexicographical_compare(lss_[lhs], lss_[rhs]); }));
}

TEST_F(BimapPartialAddedFixture, PartialAddedFindOmitsDeadPreBoundary) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;
  FinalizeShrink(lss_.added_series(), shrink_boundary);

  // Act
  const auto find_ls0 = lss_.find(ls0_);
  const auto find_ls1 = lss_.find(ls1_);
  const auto find_ls2 = lss_.find(ls2_);
  const auto find_ls3 = lss_.find(ls3_);
  const auto find_ls4 = lss_.find(ls4_);

  // Assert
  EXPECT_FALSE(find_ls0.has_value());
  EXPECT_TRUE(find_ls1.has_value());
  EXPECT_EQ(1U, *find_ls1);
  EXPECT_FALSE(find_ls2.has_value());

  ASSERT_TRUE(find_ls3.has_value());
  ASSERT_TRUE(find_ls4.has_value());
  EXPECT_EQ(3U, *find_ls3);
  EXPECT_EQ(4U, *find_ls4);
}

TEST_F(BimapPartialAddedFixture, PartialAddedResolveEmptiesDeadSlots) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;
  FinalizeShrink(lss_.added_series(), shrink_boundary);

  // Act

  // Assert
  EXPECT_EQ(0U, lss_[0].size());
  EXPECT_EQ(ls1_, lss_[1]);
  EXPECT_EQ(0U, lss_[2].size());
  EXPECT_EQ(ls3_, lss_[3]);
  EXPECT_EQ(ls4_, lss_[4]);
}

TEST_F(BimapPartialAddedFixture, PartialAddedLsIdSetKeepsAliveOnly) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;

  // Act
  FinalizeShrink(lss_.added_series(), shrink_boundary);

  // Assert
  using LsIdProxy = typename Lss::LsIdSet::value_type;

  const auto& ls_ids = lss_.ls_id_set();
  EXPECT_EQ(3U, ls_ids.size());
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{1U}));
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{3U}));
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{4U}));
  EXPECT_FALSE(ls_ids.contains(LsIdProxy{0U}));
  EXPECT_FALSE(ls_ids.contains(LsIdProxy{2U}));
}

TEST_F(BimapPartialAddedFixture, PartialAddedRecreatePrunedAtOrAfterBoundary) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;
  FinalizeShrink(lss_.added_series(), shrink_boundary);

  // Act
  const auto recreated_ls0 = lss_.find_or_emplace(ls0_);
  const auto recreated_ls2 = lss_.find_or_emplace(ls2_);
  const auto find_ls0 = lss_.find(ls0_);
  const auto find_ls2 = lss_.find(ls2_);

  // Assert
  constexpr uint32_t kExpectedRecreatedLs0Id = 5U;
  constexpr uint32_t kExpectedRecreatedLs2Id = 6U;
  EXPECT_EQ(kExpectedRecreatedLs0Id, recreated_ls0);
  EXPECT_EQ(kExpectedRecreatedLs2Id, recreated_ls2);
  ASSERT_TRUE(find_ls0.has_value());
  ASSERT_TRUE(find_ls2.has_value());
  EXPECT_EQ(recreated_ls0, *find_ls0);
  EXPECT_EQ(recreated_ls2, *find_ls2);
}

class BimapShrinkTwoFixture : public BimapCopierFixtureBase {
 protected:
  Lss lss_copy_;

  void SetUp() override {
    lss_.find_or_emplace(ls0_);
    lss_.find_or_emplace(ls1_);
    lss_.build_deferred_indexes();
  }

  void RunFinalizeShrinkWithSnapshot(const BareBones::Vector<uint32_t>& ids_for_copy) {
    const uint32_t shrink_boundary = lss_.next_item_index();
    Copier copier(lss_, lss_.sorting_index(), ids_for_copy, lss_copy_, dst_src_ids_mapping_);
    copier.copy_added_series_and_build_indexes();
    lss_.set_pending_shrink_boundary(shrink_boundary);
    lss_.finalize_copy_and_shrink(lss_copy_, dst_src_ids_mapping_);
  }
};

TEST_F(BimapShrinkTwoFixture, FinalizeShrinkSnapshotPreservesLayout) {
  // Arrange

  // Act
  RunFinalizeShrinkWithSnapshot(BareBones::Vector<uint32_t>{0U, 1U});

  // Assert
  EXPECT_TRUE(std::ranges::equal(ls0_, lss_[0]));
  EXPECT_TRUE(std::ranges::equal(ls1_, lss_[1]));
}

TEST_F(BimapShrinkTwoFixture, FinalizeShrinkSnapshotKeepsFindWorking) {
  // Arrange
  RunFinalizeShrinkWithSnapshot(BareBones::Vector<uint32_t>{0U, 1U});

  // Act
  const auto from_find_first = lss_.find(ls0_);
  const auto from_find_second = lss_.find(ls1_);

  // Assert
  ASSERT_TRUE(from_find_first.has_value());
  ASSERT_TRUE(from_find_second.has_value());
  EXPECT_EQ(0U, *from_find_first);
  EXPECT_EQ(1U, *from_find_second);
  EXPECT_TRUE(std::ranges::equal(ls0_, lss_[*from_find_first]));
  EXPECT_TRUE(std::ranges::equal(ls1_, lss_[*from_find_second]));
}

TEST_F(BimapShrinkTwoFixture, ShrunkTwoSeriesCountMatchesIndices) {
  // Arrange

  // Act
  RunFinalizeShrinkWithSnapshot(BareBones::Vector<uint32_t>{0U, 1U});

  // Assert
  EXPECT_EQ(2U, lss_.items_count());
  EXPECT_EQ(2U, lss_.next_item_index());
}

}  // namespace
