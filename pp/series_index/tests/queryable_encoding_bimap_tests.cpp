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

class BimapFixedStateFixture : public BimapFixture {
 protected:
  const LabelViewSet ls0_{{"job", "a"}};
  const LabelViewSet ls1_{{"job", "b"}};
  const LabelViewSet ls2_{{"job", "c"}};
  const LabelViewSet ls3_{{"job", "d"}};
  const LabelViewSet ls4_{{"job", "e"}};

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

  [[nodiscard]] std::vector<uint32_t> ls_logical_ids_in_set_order() const {
    std::vector<uint32_t> ids;
    ids.reserve(lss_.ls_id_set().size());
    for (const auto id : lss_.ls_id_set()) {
      ids.emplace_back(static_cast<uint32_t>(id));
    }
    return ids;
  }
};

TEST_F(BimapFixedStateFixture, LssInitialState) {
  // Arrange

  // Act

  // Assert
  EXPECT_EQ(5U, lss_.items_count());
  EXPECT_EQ(5U, lss_.next_item_index());

  EXPECT_TRUE(std::ranges::equal(lss_.added_series(), std::vector<uint32_t>{1, 3, 4}));
}

TEST_F(BimapFixedStateFixture, FixedStateKeepsSeriesCount) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;

  // Act
  lss_.set_pending_shrink_boundary(shrink_boundary);

  // Assert
  EXPECT_EQ(5U, lss_.items_count());
  EXPECT_EQ(5U, lss_.next_item_index());
}

TEST_F(BimapFixedStateFixture, FixedStateResolveBehavesLikeNormal) {
  // Arrange
  auto before_shrink = lss_[2];

  constexpr uint32_t shrink_boundary = 3U;
  lss_.set_pending_shrink_boundary(shrink_boundary);

  // Act
  const auto after_shrink = lss_[2];

  // Assert
  EXPECT_EQ(before_shrink, after_shrink);
}

TEST_F(BimapFixedStateFixture, FixedStateFindHidesPreBoundarySeries) {
  // Arrange
  const auto from_find_before_fixed = lss_.find(ls2_);
  constexpr uint32_t shrink_boundary = 3U;
  lss_.set_pending_shrink_boundary(shrink_boundary);

  // Act
  const auto from_find = lss_.find(ls2_);

  // Assert
  ASSERT_TRUE(from_find_before_fixed.has_value());
  EXPECT_FALSE(from_find.has_value());
}

TEST_F(BimapFixedStateFixture, NormalStateFillsNonaddedSeries) {
  // Arrange

  // Act
  const auto ls_id = lss_.find_or_emplace(ls0_);

  // Assert
  EXPECT_EQ(ls_id, 0U);
  EXPECT_EQ(ls0_, lss_[0]);
  EXPECT_TRUE(lss_.added_series()[0]);
}

TEST_F(BimapFixedStateFixture, FixedStateAppendPastBoundary) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;
  lss_.set_pending_shrink_boundary(shrink_boundary);

  // Act
  const auto ls_id = lss_.find_or_emplace(ls0_);

  // Assert
  EXPECT_EQ(ls_id, 5);
  EXPECT_EQ(lss_[0], lss_[5]);

  EXPECT_TRUE(lss_.added_series()[5]);
  EXPECT_FALSE(lss_.added_series()[0]);
}

TEST_F(BimapFixedStateFixture, FixedStatePrunesLsIdSet) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;
  const LabelViewSet ls5{{"job", "f"}};

  lss_.set_pending_shrink_boundary(shrink_boundary);

  // Act
  const auto new_id = lss_.find_or_emplace(ls5);
  const auto& ls_ids = lss_.ls_id_set();

  // Assert
  EXPECT_EQ(new_id, 5U);
  EXPECT_EQ(ls_ids.size(), 4U);

  using LsIdProxy = typename Lss::LsIdSet::value_type;
  EXPECT_FALSE(ls_ids.contains(LsIdProxy{0U}));
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{1U}));
  EXPECT_FALSE(ls_ids.contains(LsIdProxy{2U}));
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{3U}));
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{4U}));
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{5U}));
}

TEST_F(BimapFixedStateFixture, FixedStateLsIdSetIsSorted) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;
  const LabelViewSet ls5{{"job", "f"}};

  lss_.set_pending_shrink_boundary(shrink_boundary);

  // Act
  std::ignore = lss_.find_or_emplace(ls5);

  // Assert
  EXPECT_TRUE(
      std::ranges::is_sorted(lss_.ls_id_set(), [this](uint32_t lhs, uint32_t rhs) { return std::ranges::lexicographical_compare(lss_[lhs], lss_[rhs]); }));
}

TEST_F(BimapFixedStateFixture, FixedStateSortingIndexSortKeepsOrder) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;
  lss_.set_pending_shrink_boundary(shrink_boundary);

  std::vector<uint32_t> ids = {3U, 1U, 4U};
  const std::vector<uint32_t> expected_ids = {1U, 3U, 4U};

  // Act
  lss_.sorting_index().sort(ids);

  // Assert
  EXPECT_TRUE(std::ranges::is_sorted(expected_ids, [this](uint32_t lhs, uint32_t rhs) { return std::ranges::lexicographical_compare(lss_[lhs], lss_[rhs]); }));
  EXPECT_EQ(ids, expected_ids);
}

class BimapCopierFixture : public BimapFixture {
 protected:
  const LabelViewSet ls0_{{"job", "a"}};
  const LabelViewSet ls1_{{"job", "b"}};
  const LabelViewSet ls2_{{"job", "c"}};
  const LabelViewSet ls3_{{"job", "d"}};
  const LabelViewSet ls4_{{"job", "e"}};
  BareBones::Vector<uint32_t> dst_src_ids_mapping_;

  void SetUp() override {
    lss_.find_or_emplace(ls0_);
    lss_.find_or_emplace(ls1_);
    lss_.find_or_emplace(ls2_);
    lss_.find_or_emplace(ls3_);
    lss_.find_or_emplace(ls4_);
    lss_.build_deferred_indexes();
  }
};

TEST_F(BimapCopierFixture, CopyFromEmptySourceLeavesEmpty) {
  // Arrange
  Lss empty_lss;
  Lss lss_copy;
  BareBones::Vector<uint32_t> dst_src_ids_mapping;
  Copier copier(empty_lss, empty_lss.sorting_index(), empty_lss.added_series(), lss_copy, dst_src_ids_mapping);

  // Act
  copier.copy_added_series_and_build_indexes();

  // Assert
  EXPECT_EQ(0U, lss_copy.items_count());
  EXPECT_EQ(0U, dst_src_ids_mapping.size());
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

TEST_F(BimapCopierFixture, FinalizeShrinkKeepsTrie) {
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

class BimapShrinkedStateFixture : public BimapFixture {
 protected:
  static constexpr uint32_t kShrinkBoundary = 3U;

  const LabelViewSet ls0_{{"job", "a"}};
  const LabelViewSet ls1_{{"job", "b"}};
  const LabelViewSet ls2_{{"job", "c"}};
  const LabelViewSet ls3_{{"job", "d"}};
  const LabelViewSet ls4_{{"job", "e"}};
  const LabelViewSet ls5_{{"job", "f"}};

  BareBones::Vector<uint32_t> dst_src_ids_mapping_;
  Lss lss_copy_;

  void SetUp() override {
    Lss seeded_lss;
    seeded_lss.find_or_emplace(ls0_);
    seeded_lss.find_or_emplace(ls1_);
    seeded_lss.find_or_emplace(ls2_);
    seeded_lss.find_or_emplace(ls3_);
    seeded_lss.find_or_emplace(ls4_);
    seeded_lss.build_deferred_indexes();

    Copier copier(seeded_lss, seeded_lss.sorting_index(), seeded_lss.added_series(), lss_, dst_src_ids_mapping_);
    copier.copy_added_series_and_build_indexes();

    // Mixed state: ids 1, 3 and 4 are active; ids 0 and 2 are hidden before the shrink boundary.
    [[maybe_unused]] const auto touched_ls1 = lss_.find_or_emplace(ls1_);
    [[maybe_unused]] const auto touched_ls3 = lss_.find_or_emplace(ls3_);
    [[maybe_unused]] const auto touched_ls4 = lss_.find_or_emplace(ls4_);
    lss_.build_deferred_indexes();

    dst_src_ids_mapping_.clear();
    Copier shrink_copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy_, dst_src_ids_mapping_);
    shrink_copier.copy_added_series_and_build_indexes();
    lss_.set_pending_shrink_boundary(kShrinkBoundary);
    lss_.finalize_copy_and_shrink(lss_copy_, dst_src_ids_mapping_);
  }
};

TEST_F(BimapShrinkedStateFixture, ShrinkHidesNonAddedSeries) {
  // Arrange

  // Act

  // Assert
  EXPECT_EQ(3U, lss_.items_count());
  EXPECT_EQ(5U, lss_.next_item_index());
}

TEST_F(BimapShrinkedStateFixture, ShrinkResolveSkipsNonAddedSeries) {
  // Arrange

  // Act

  // Assert
  EXPECT_EQ(0U, lss_[0].size());
  EXPECT_EQ(ls1_, lss_[1]);
  EXPECT_EQ(0U, lss_[2].size());
  EXPECT_EQ(ls3_, lss_[3]);
  EXPECT_EQ(ls4_, lss_[4]);
}

TEST_F(BimapShrinkedStateFixture, ShrinkFindOrEmplaceRecreatesNonAddedSeriesAtBoundary) {
  // Arrange

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

TEST_F(BimapShrinkedStateFixture, ShrinkFindOrEmplaceNewSeriesAppendsAtBoundary) {
  // Arrange

  // Act
  const auto ls_id = lss_.find_or_emplace(ls5_);
  const auto from_find = lss_.find(ls5_);

  // Assert
  EXPECT_EQ(5U, ls_id);
  ASSERT_TRUE(from_find.has_value());
  EXPECT_EQ(ls_id, *from_find);
  EXPECT_EQ(ls5_, lss_[ls_id]);
}

TEST_F(BimapShrinkedStateFixture, ShrinkFindOrEmplaceExistingSeries) {
  // Arrange

  // Act
  const auto ls_id = lss_.find_or_emplace(ls1_);
  const auto from_find = lss_.find(ls1_);

  // Assert
  EXPECT_EQ(1U, ls_id);
  ASSERT_TRUE(from_find.has_value());
  EXPECT_EQ(ls_id, *from_find);
  EXPECT_EQ(ls1_, lss_[ls_id]);
}

TEST_F(BimapShrinkedStateFixture, ShrinkLsIdSetKeepsAliveOnly) {
  // Arrange

  // Act

  // Assert
  const auto& ls_ids = lss_.ls_id_set();

  EXPECT_EQ(3U, ls_ids.size());

  using LsIdProxy = typename Lss::LsIdSet::value_type;
  EXPECT_FALSE(ls_ids.contains(LsIdProxy{0U}));
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{1U}));
  EXPECT_FALSE(ls_ids.contains(LsIdProxy{2U}));
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{3U}));
  EXPECT_TRUE(ls_ids.contains(LsIdProxy{4U}));
}

TEST_F(BimapShrinkedStateFixture, ShrinkLsIdSetIsSorted) {
  // Arrange

  // Act

  // Assert
  EXPECT_TRUE(
      std::ranges::is_sorted(lss_.ls_id_set(), [this](uint32_t lhs, uint32_t rhs) { return std::ranges::lexicographical_compare(lss_[lhs], lss_[rhs]); }));
}

TEST_F(BimapShrinkedStateFixture, ShrinkSortingIndexSortKeepsOrder) {
  // Arrange
  std::vector<uint32_t> ids = {4U, 3U, 1U};
  const std::vector<uint32_t> expected_ids = {1U, 3U, 4U};

  // Act
  lss_.sorting_index().sort(ids);

  // Assert
  EXPECT_TRUE(std::ranges::is_sorted(expected_ids, [this](uint32_t lhs, uint32_t rhs) { return std::ranges::lexicographical_compare(lss_[lhs], lss_[rhs]); }));
  EXPECT_EQ(ids, expected_ids);
}

TEST_F(BimapShrinkedStateFixture, ShrunkStateCorrectlyResolvesSeries) {
  // Arrange

  // Act
  const auto ls3_id = lss_.find(ls3_);
  const auto ls4_id = lss_.find(ls4_);

  // Assert
  ASSERT_TRUE(ls3_id.has_value());
  ASSERT_TRUE(ls4_id.has_value());
  EXPECT_EQ(kShrinkBoundary, *ls3_id);
  EXPECT_EQ(kShrinkBoundary + 1, *ls4_id);
  EXPECT_EQ(ls3_, lss_[*ls3_id]);
  EXPECT_EQ(ls4_, lss_[*ls4_id]);
}

TEST_F(BimapShrinkedStateFixture, FindOmitsNonAddedPreBoundarySeries) {
  // Arrange

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
  EXPECT_EQ(3U, *find_ls3);

  ASSERT_TRUE(find_ls4.has_value());
  EXPECT_EQ(4U, *find_ls4);
}

}  // namespace
