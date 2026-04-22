#include <gmock/gmock-matchers.h>
#include <gtest/gtest.h>

#include "primitives/label_set.h"
#include "primitives/snug_composites.h"
#include "series_index/prometheus/tsdb/index/index_write_context.h"
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

TEST_F(BimapFixture, EmplaceLabelSet) {
  // Arrange

  // Act
  lss_.find_or_emplace(LabelViewSet{{"job", "cron"}});
  const auto name_id = lss_.trie_index().names_trie().lookup("job");

  // Assert
  EXPECT_TRUE(name_id);
  EXPECT_NE(nullptr, lss_.reverse_index().get(*name_id));

  const auto values_trie = lss_.trie_index().values_trie(*name_id);
  ASSERT_NE(nullptr, values_trie);
  EXPECT_TRUE(values_trie->lookup("cron"));
}

TEST_F(BimapFixture, EmptyCompositeHasZeroSize) {
  // Arrange
  typename Lss::Base::value_type empty;

  // Act

  // Assert
  EXPECT_EQ(0U, empty.size());
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
  EXPECT_EQ(2U, lss_.series_count());
}

TEST_F(BimapFixture, InsertedSeriesUpdatesMaxItemIndex) {
  // Arrange
  lss_.find_or_emplace(LabelViewSet{{"job", "cron"}});
  lss_.find_or_emplace(LabelViewSet{{"job", "worker"}});

  // Act

  // Assert
  EXPECT_EQ(2U, lss_.max_item_index());
}

TEST_F(BimapFixture, FixedStateFindKeepsTouchedPreBoundaryId) {
  // Arrange
  const auto ls1 = LabelViewSet{{"job", "a"}};
  lss_.find_or_emplace(ls1);
  constexpr uint32_t shrink_boundary = 1U;
  lss_.set_pending_shrink_boundary(shrink_boundary);

  // Act
  const auto from_find = lss_.find(ls1);

  // Assert
  ASSERT_TRUE(from_find.has_value());
  EXPECT_EQ(0U, *from_find);
  EXPECT_EQ(ls1, lss_[0]);
}

TEST_F(BimapFixture, FixedStateFindOrEmplaceKeepsPreBoundaryId) {
  // Arrange
  const auto ls1 = LabelViewSet{{"job", "a"}};
  const auto ls2 = LabelViewSet{{"job", "b"}};
  lss_.find_or_emplace(ls1);
  lss_.find_or_emplace(ls2);
  const uint32_t shrink_boundary = lss_.max_item_index();
  lss_.set_pending_shrink_boundary(shrink_boundary);

  // Act
  const auto from_find_before = lss_.find(ls1);
  const auto returned_id = lss_.find_or_emplace(ls1);
  const auto from_find_after = lss_.find(ls1);

  // Assert
  ASSERT_TRUE(from_find_before.has_value());
  EXPECT_EQ(0U, *from_find_before);
  EXPECT_EQ(*from_find_before, returned_id);
  ASSERT_TRUE(from_find_after.has_value());
  EXPECT_EQ(returned_id, *from_find_after);
}

TEST_F(BimapFixture, FixedStateFindOrEmplaceNewSeriesGetsVisibleId) {
  // Arrange
  const auto ls1 = LabelViewSet{{"job", "a"}};
  const auto ls2 = LabelViewSet{{"job", "b"}};
  const auto ls3 = LabelViewSet{{"job", "c"}};
  constexpr uint32_t shrink_boundary = 2U;
  lss_.find_or_emplace(ls1);
  lss_.find_or_emplace(ls2);
  lss_.set_pending_shrink_boundary(shrink_boundary);

  // Act
  const auto new_id = lss_.find_or_emplace(ls3);
  const auto from_find = lss_.find(ls3);

  // Assert
  EXPECT_GE(new_id, shrink_boundary);
  ASSERT_TRUE(from_find.has_value());
  EXPECT_EQ(new_id, *from_find);
  EXPECT_EQ(ls3, lss_[new_id]);
}

TEST_F(BimapFixture, FixedStateFindOrEmplaceMarksAddedSlot) {
  // Arrange
  const auto ls1 = LabelViewSet{{"job", "a"}};
  const auto ls2 = LabelViewSet{{"job", "b"}};
  const auto ls3 = LabelViewSet{{"job", "c"}};
  constexpr uint32_t shrink_boundary = 2U;
  lss_.find_or_emplace(ls1);
  lss_.find_or_emplace(ls2);
  lss_.set_pending_shrink_boundary(shrink_boundary);

  // Act
  const auto new_id = lss_.find_or_emplace(ls3);

  // Assert
  ASSERT_LT(new_id, lss_.added_series().size());
  EXPECT_TRUE(lss_.added_series()[new_id]);
}

class BimapCopierFixture : public BimapFixture {
 protected:
  static constexpr uint32_t kFirstSeriesId = 0U;
  static constexpr uint32_t kSecondSeriesId = 1U;
  const LabelViewSet copied_ls1_{{"job", "cron"}, {"key", ""}, {"process", "php1"}};
  const LabelViewSet copied_ls2_{{"job", "cron"}, {"key", ""}, {"process", "php"}};
  BareBones::Vector<uint32_t> dst_src_ids_mapping_;

  void SeedTwoSeriesForCopy() {
    lss_.find_or_emplace(copied_ls1_);
    lss_.find_or_emplace(copied_ls2_);
    lss_.build_deferred_indexes();
  }

  void CopySeriesByIds(const BareBones::Vector<uint32_t>& ids_for_copy, Lss& lss_copy) {
    dst_src_ids_mapping_.clear();
    Copier copier(lss_, lss_.sorting_index(), ids_for_copy, lss_copy, dst_src_ids_mapping_);
    copier.copy_added_series_and_build_indexes();
  }

  void SeedFiveKeySeries() {
    lss_.find_or_emplace(LabelViewSet{{"job", "cron"}, {"key", "1"}, {"process", "php"}});
    lss_.find_or_emplace(LabelViewSet{{"job", "cron"}, {"key", "2"}, {"process", "php"}});
    lss_.find_or_emplace(LabelViewSet{{"job", "cron"}, {"key", "3"}, {"process", "php"}});
    lss_.find_or_emplace(LabelViewSet{{"job", "cron"}, {"key", "4"}, {"process", "php"}});
    lss_.find_or_emplace(LabelViewSet{{"job", "cron"}, {"key", "5"}, {"process", "php"}});
    lss_.build_deferred_indexes();
  }
};

TEST_F(BimapCopierFixture, CopyFromEmptySourceLeavesEmpty) {
  // Arrange
  Lss lss_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);

  // Act
  copier.copy_added_series_and_build_indexes();

  // Assert
  EXPECT_EQ(0U, lss_copy.series_count());
  EXPECT_EQ(0U, dst_src_ids_mapping_.size());
}

TEST_F(BimapCopierFixture, CopyKeepsSeriesCount) {
  // Arrange
  SeedTwoSeriesForCopy();
  const BareBones::Vector ids_for_copy{kFirstSeriesId, kSecondSeriesId};
  Lss lss_copy;

  // Act
  CopySeriesByIds(ids_for_copy, lss_copy);

  // Assert
  EXPECT_EQ(2U, lss_copy.series_count());
}

TEST_F(BimapCopierFixture, CopyFindsCopiedSeries) {
  // Arrange
  SeedTwoSeriesForCopy();
  const BareBones::Vector ids_for_copy{kFirstSeriesId, kSecondSeriesId};
  Lss lss_copy;

  // Act
  CopySeriesByIds(ids_for_copy, lss_copy);

  // Assert
  EXPECT_TRUE(lss_copy.find(copied_ls1_).has_value());
  EXPECT_TRUE(lss_copy.find(copied_ls2_).has_value());
}

TEST_F(BimapCopierFixture, CopyBuildsTrieAndPostingIndexes) {
  // Arrange
  SeedTwoSeriesForCopy();
  const BareBones::Vector ids_for_copy{kFirstSeriesId, kSecondSeriesId};
  Lss lss_copy;

  // Act
  CopySeriesByIds(ids_for_copy, lss_copy);

  // Assert
  EXPECT_EQ(2U, lss_copy.reverse_index().names_count());
  EXPECT_EQ(2U, lss_copy.ls_id_set().size());
  EXPECT_FALSE(lss_copy.ls_id_set().empty());
  EXPECT_TRUE(lss_copy.trie_index().names_trie().lookup("job"));
}

TEST_F(BimapCopierFixture, CopyPreservesSourceIdMapping) {
  // Arrange
  SeedTwoSeriesForCopy();
  const BareBones::Vector ids_for_copy{kFirstSeriesId, kSecondSeriesId};
  Lss lss_copy;

  // Act
  CopySeriesByIds(ids_for_copy, lss_copy);

  // Assert
  EXPECT_EQ(ids_for_copy, dst_src_ids_mapping_);
}

TEST_F(BimapCopierFixture, CopyPreservesLexicographicOrder) {
  // Arrange
  SeedFiveKeySeries();
  Lss lss_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);

  // Act
  copier.copy_added_series_and_build_indexes();

  // Assert
  EXPECT_TRUE(std::ranges::is_sorted(lss_copy.ls_id_set(),
                                     [&](const auto idl, const auto idr) { return std::ranges::lexicographical_compare(lss_copy[idl], lss_copy[idr]); }));
  EXPECT_EQ((BareBones::Vector{0U, 1U, 2U, 3U, 4U}), dst_src_ids_mapping_);
}

TEST_F(BimapCopierFixture, CopySubsetKeepsOrderAndMapping) {
  // Arrange
  const auto label_set1 = LabelViewSet{{"job", "cron"}, {"key", "1"}, {"process", "php"}};
  const auto label_set3 = LabelViewSet{{"job", "cron"}, {"key", "3"}, {"process", "php"}};
  const auto label_set5 = LabelViewSet{{"job", "cron"}, {"key", "5"}, {"process", "php"}};
  SeedFiveKeySeries();

  Lss lss_copy;
  const BareBones::Vector ids_for_copy{0U, 2U, 4U};
  Copier copier(lss_, lss_.sorting_index(), ids_for_copy, lss_copy, dst_src_ids_mapping_);

  // Act
  copier.copy_added_series_and_build_indexes();

  // Assert
  ASSERT_EQ(3U, lss_copy.series_count());
  EXPECT_EQ(label_set1, lss_copy[0]);
  EXPECT_EQ(label_set3, lss_copy[1]);
  EXPECT_EQ(label_set5, lss_copy[2]);
  EXPECT_EQ(ids_for_copy, dst_src_ids_mapping_);
}

TEST_F(BimapCopierFixture, CopyOfCopyWithNoNewAddedIsEmpty) {
  // Arrange
  Lss lss_copy;
  Lss lss_copy_of_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);
  Copier copier2(lss_copy, lss_copy.sorting_index(), lss_copy.added_series(), lss_copy_of_copy, dst_src_ids_mapping_);

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

  // Assert
  EXPECT_EQ(0U, lss_copy_of_copy.series_count());
  EXPECT_FALSE(lss_copy_of_copy.find(label_set));
  EXPECT_FALSE(lss_copy_of_copy.find(label_set2));
  EXPECT_FALSE(lss_copy_of_copy.find(label_set3));
  EXPECT_TRUE(dst_src_ids_mapping_.empty());
}

TEST_F(BimapCopierFixture, CopyOfCopyInsertGetsFirstId) {
  // Arrange
  Lss lss_copy;
  Lss lss_copy_of_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);
  Copier copier2(lss_copy, lss_copy.sorting_index(), lss_copy.added_series(), lss_copy_of_copy, dst_src_ids_mapping_);

  const auto label_set = LabelViewSet{{"job", "cron"}, {"key", ""}, {"process", "php"}};
  const auto label_set2 = LabelViewSet{{"job", "cron"}, {"key", ""}, {"process", "php1"}};
  const auto label_set3 = LabelViewSet{{"server", "localhost"}};

  lss_.find_or_emplace(label_set);
  lss_.find_or_emplace(label_set2);
  lss_.find_or_emplace(label_set3);
  lss_.build_deferred_indexes();

  copier.copy_added_series_and_build_indexes();
  lss_copy.build_deferred_indexes();
  copier2.copy_added_series_and_build_indexes();

  // Act
  const auto ls_id = lss_copy_of_copy.find_or_emplace(label_set3);

  // Assert
  EXPECT_EQ(0U, ls_id);
  EXPECT_TRUE(lss_copy_of_copy.find(label_set3).has_value());
  EXPECT_EQ(1U, lss_copy_of_copy.series_count());
}

TEST_F(BimapCopierFixture, CopyOfCopyInsertBuildsIndexesForNewSeries) {
  // Arrange
  Lss lss_copy;
  Lss lss_copy_of_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);
  Copier copier2(lss_copy, lss_copy.sorting_index(), lss_copy.added_series(), lss_copy_of_copy, dst_src_ids_mapping_);

  const auto label_set = LabelViewSet{{"job", "cron"}, {"key", ""}, {"process", "php"}};
  const auto label_set2 = LabelViewSet{{"job", "cron"}, {"key", ""}, {"process", "php1"}};
  const auto label_set3 = LabelViewSet{{"server", "localhost"}};

  lss_.find_or_emplace(label_set);
  lss_.find_or_emplace(label_set2);
  lss_.find_or_emplace(label_set3);
  lss_.build_deferred_indexes();

  copier.copy_added_series_and_build_indexes();
  lss_copy.build_deferred_indexes();
  copier2.copy_added_series_and_build_indexes();

  // Act
  [[maybe_unused]] const auto ls_id = lss_copy_of_copy.find_or_emplace(label_set3);

  // Assert
  EXPECT_EQ(1U, lss_copy_of_copy.reverse_index().names_count());
  EXPECT_EQ(1U, lss_copy_of_copy.ls_id_set().size());
  EXPECT_FALSE(lss_copy_of_copy.trie_index().names_trie().lookup("job"));
  EXPECT_TRUE(lss_copy_of_copy.trie_index().names_trie().lookup("server"));
}

TEST_F(BimapCopierFixture, FinalizeShrinkKeepsTrieAfterTwoSeries) {
  // Arrange
  const auto ls1 = LabelViewSet{{"job", "a"}};
  const auto ls2 = LabelViewSet{{"job", "b"}};
  lss_.find_or_emplace(ls1);
  lss_.find_or_emplace(ls2);
  lss_.build_deferred_indexes();

  const uint32_t shrink_boundary = lss_.max_item_index();
  Lss lss_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);
  copier.copy_added_series_and_build_indexes();

  // Act
  lss_.set_pending_shrink_boundary(shrink_boundary);
  lss_.finalize_copy_and_shrink(lss_copy, dst_src_ids_mapping_);

  // Assert
  EXPECT_TRUE(lss_.trie_index().names_trie().lookup("job"));
  ASSERT_NE(nullptr, lss_.trie_index().values_trie(*lss_.trie_index().names_trie().lookup("job")));
  EXPECT_EQ(ls1, lss_[0]);
  EXPECT_EQ(ls2, lss_[1]);
}

class BimapShrinkFixture : public BimapCopierFixture {
 protected:
  LabelViewSet ls1_{{"job", "a"}};
  LabelViewSet ls2_{{"job", "b"}};
  LabelViewSet ls3_{{"job", "c"}};

  void SetUp() override {
    BimapCopierFixture::SetUp();
    lss_.find_or_emplace(ls1_);
    lss_.find_or_emplace(ls2_);
    lss_.find_or_emplace(ls3_);
    lss_.build_deferred_indexes();
  }
};

TEST_F(BimapShrinkFixture, FinalizeShrinkMapsSeriesInOrder) {
  // Arrange
  const uint32_t shrink_boundary = lss_.max_item_index();
  Lss lss_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);
  copier.copy_added_series_and_build_indexes();

  // Act
  lss_.set_pending_shrink_boundary(shrink_boundary);
  lss_.finalize_copy_and_shrink(lss_copy, dst_src_ids_mapping_);

  // Assert
  EXPECT_EQ(ls1_, lss_[0]);
  EXPECT_EQ(ls2_, lss_[1]);
  EXPECT_EQ(ls3_, lss_[2]);
}

TEST_F(BimapShrinkFixture, ShrunkStateSeriesCountMatchesStorage) {
  // Arrange
  const uint32_t shrink_boundary = lss_.max_item_index();
  Lss lss_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);
  copier.copy_added_series_and_build_indexes();

  // Act
  lss_.set_pending_shrink_boundary(shrink_boundary);
  lss_.finalize_copy_and_shrink(lss_copy, dst_src_ids_mapping_);

  // Assert
  EXPECT_EQ(3U, lss_.series_count());
  EXPECT_EQ(3U, lss_.max_item_index());
}

TEST_F(BimapShrinkFixture, IndexWriteContextDedupesSymbolsAfterFullShrink) {
  // Arrange
  const uint32_t shrink_boundary = lss_.max_item_index();
  Lss lss_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);
  copier.copy_added_series_and_build_indexes();
  lss_.set_pending_shrink_boundary(shrink_boundary);
  lss_.finalize_copy_and_shrink(lss_copy, dst_src_ids_mapping_);

  // Act
  const auto ctx = series_index::prometheus::tsdb::index::IndexWriteContext<Lss>{lss_};
  std::vector<std::string> symbols;
  ctx.for_each_symbol([&](uint32_t /*symbol_ref*/, std::string_view s) { symbols.emplace_back(s); });

  // Assert
  EXPECT_THAT(symbols, testing::ElementsAre("", "a", "b", "c", "job"));
}

TEST_F(BimapShrinkFixture, IndexWriteContextResolvesRefsAfterFullShrink) {
  // Arrange
  const uint32_t shrink_boundary = lss_.max_item_index();
  Lss lss_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);
  copier.copy_added_series_and_build_indexes();
  lss_.set_pending_shrink_boundary(shrink_boundary);
  lss_.finalize_copy_and_shrink(lss_copy, dst_src_ids_mapping_);
  const auto ctx = series_index::prometheus::tsdb::index::IndexWriteContext<Lss>{lss_};

  // Act
  const auto labels0 = lss_[0];
  const auto labels1 = lss_[1];
  const auto labels2 = lss_[2];
  const auto name_ref0 = ctx.symbol_ref_for_name_for_series(0, labels0.begin().name_id());
  const auto name_ref1 = ctx.symbol_ref_for_name_for_series(1, labels1.begin().name_id());
  const auto name_ref2 = ctx.symbol_ref_for_name_for_series(2, labels2.begin().name_id());
  const auto value_ref0 = ctx.symbol_ref_for_value_for_series(0, labels0.begin().name_id(), labels0.begin().value_id());
  const auto value_ref1 = ctx.symbol_ref_for_value_for_series(1, labels1.begin().name_id(), labels1.begin().value_id());
  const auto value_ref2 = ctx.symbol_ref_for_value_for_series(2, labels2.begin().name_id(), labels2.begin().value_id());

  // Assert
  EXPECT_EQ(name_ref0, name_ref1);
  EXPECT_EQ(name_ref1, name_ref2);
  EXPECT_NE(value_ref0, value_ref1);
  EXPECT_NE(value_ref1, value_ref2);
}

class BimapFixedStateFixture : public BimapCopierFixture {
 protected:
  static constexpr uint32_t kPendingShrinkBoundary = 2;
  LabelViewSet ls1_{{"job", "a"}};
  LabelViewSet ls2_{{"job", "b"}};
  LabelViewSet ls3_{{"job", "c"}};

  void SetUp() override {
    BimapCopierFixture::SetUp();
    lss_.find_or_emplace(ls1_);
    lss_.find_or_emplace(ls2_);
    lss_.set_pending_shrink_boundary(kPendingShrinkBoundary);
    lss_.find_or_emplace(ls3_);
  }
};

TEST_F(BimapFixedStateFixture, FixedStateFindMatchesFindOrEmplace) {
  // Arrange

  // Act
  const auto from_find = lss_.find(ls3_);
  const auto from_find_or_emplace = lss_.find_or_emplace(ls3_);

  // Assert
  ASSERT_TRUE(from_find.has_value());
  EXPECT_GE(*from_find, kPendingShrinkBoundary);
  EXPECT_EQ(from_find_or_emplace, *from_find);
  EXPECT_TRUE(std::ranges::equal(ls3_, lss_[*from_find]));
}

TEST_F(BimapFixedStateFixture, FixedStateSeriesCountMatchesThree) {
  // Arrange

  // Act

  // Assert
  EXPECT_EQ(3U, lss_.series_count());
  EXPECT_EQ(3U, lss_.max_item_index());
}

TEST_F(BimapFixedStateFixture, FixedStateBracketExposesVisibleSeries) {
  // Arrange

  // Act
  const auto id = lss_.find(ls3_);

  // Assert
  ASSERT_TRUE(id.has_value());
  EXPECT_GE(*id, kPendingShrinkBoundary);
  EXPECT_TRUE(std::ranges::equal(ls1_, lss_[0]));
  EXPECT_TRUE(std::ranges::equal(ls2_, lss_[1]));
  EXPECT_TRUE(std::ranges::equal(ls3_, lss_[2]));
  EXPECT_TRUE(std::ranges::equal(ls3_, lss_[*id]));
}

class BimapFixedPendingFixture : public BimapCopierFixture {
 protected:
  static constexpr uint32_t kPendingShrinkBoundary = 2;
  LabelViewSet ls1_{{"job", "a"}};
  LabelViewSet ls2_{{"job", "b"}};
  LabelViewSet ls3_{{"job", "c"}};

  void SetUp() override {
    BimapCopierFixture::SetUp();
    lss_.find_or_emplace(ls1_);
    lss_.find_or_emplace(ls2_);
    lss_.set_pending_shrink_boundary(kPendingShrinkBoundary);
  }
};

TEST_F(BimapFixedPendingFixture, FixedStateInsertTailGetsBoundaryOrAbove) {
  // Arrange

  // Act
  const auto id = lss_.find_or_emplace(ls3_);

  // Assert
  EXPECT_GE(id, kPendingShrinkBoundary);
  const auto from_find = lss_.find(ls3_);
  ASSERT_TRUE(from_find.has_value());
  EXPECT_EQ(id, *from_find);
  EXPECT_TRUE(std::ranges::equal(ls3_, lss_[id]));
}

TEST_F(BimapFixedPendingFixture, FixedStateTwoSeriesCountMatchesStorage) {
  // Arrange

  // Act

  // Assert
  EXPECT_EQ(2U, lss_.series_count());
  EXPECT_EQ(2U, lss_.max_item_index());
}

class BimapFiveSeriesFixture : public BimapCopierFixture {
 protected:
  LabelViewSet ls1_{{"job", "a"}};
  LabelViewSet ls2_{{"job", "b"}};
  LabelViewSet ls3_{{"job", "c"}};
  LabelViewSet ls4_{{"job", "d"}};
  LabelViewSet ls5_{{"job", "e"}};
  LabelViewSet ls6_{{"job", "f"}};
  Lss lss_copy_;

  void SetUp() override {
    BimapCopierFixture::SetUp();
    lss_.find_or_emplace(ls1_);
    lss_.find_or_emplace(ls2_);
    lss_.find_or_emplace(ls3_);
    lss_.find_or_emplace(ls4_);
    lss_.find_or_emplace(ls5_);
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
};

TEST_F(BimapFiveSeriesFixture, ShrinkFindOrEmplaceAddsAtBoundary) {
  // Arrange
  const uint32_t shrink_boundary = lss_.max_item_index();
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
  const auto from_find_ls2 = lss_.find(ls2_);

  // Assert
  EXPECT_EQ(ls1_, lss_[0]);
  EXPECT_EQ(ls2_, lss_[1]);
  EXPECT_EQ(ls3_, lss_[2]);
  EXPECT_EQ(ls4_, lss_[3]);
  EXPECT_EQ(ls5_, lss_[4]);
  ASSERT_TRUE(from_find_ls2.has_value());
  EXPECT_EQ(1U, *from_find_ls2);
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
  const auto existing_id = lss_.find_or_emplace(ls2_);
  const auto from_find_after = lss_.find(ls2_);

  // Assert
  EXPECT_EQ(1U, existing_id);
  ASSERT_TRUE(from_find_after.has_value());
  EXPECT_EQ(existing_id, *from_find_after);
  EXPECT_EQ(ls2_, lss_[existing_id]);
}

TEST_F(BimapFiveSeriesFixture, ShrunkReinsertMarksAddedAndLsIdSet) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;
  constexpr uint32_t expected_existing_id = 1U;
  FinalizeShrink(lss_.added_series(), shrink_boundary);

  // Act
  const auto new_id = lss_.find_or_emplace(ls2_);

  // Assert
  using LsIdProxy = typename Lss::LsIdSet::value_type;

  ASSERT_EQ(expected_existing_id, new_id);
  ASSERT_LT(new_id, lss_.added_series().size());
  EXPECT_TRUE(lss_.added_series()[new_id]);
  EXPECT_TRUE(lss_.ls_id_set().contains(LsIdProxy{new_id}));
}

TEST_F(BimapFiveSeriesFixture, ShrinkFullCopyKeepsSeriesLayout) {
  // Arrange
  const uint32_t shrink_boundary = lss_.max_item_index();
  FinalizeShrink(lss_.added_series(), shrink_boundary);

  // Act
  const auto from_find = lss_.find(ls2_);

  // Assert
  EXPECT_EQ(ls1_, lss_[0]);
  EXPECT_EQ(ls2_, lss_[1]);
  EXPECT_EQ(ls3_, lss_[2]);
  EXPECT_EQ(ls4_, lss_[3]);
  EXPECT_EQ(ls5_, lss_[4]);
  ASSERT_TRUE(from_find.has_value());
  EXPECT_EQ(1U, *from_find);
}

TEST_F(BimapFiveSeriesFixture, ShrinkFullCopyLsIdSetHasLogicalIds) {
  // Arrange
  const uint32_t shrink_boundary = lss_.max_item_index();
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
  const uint32_t shrink_boundary = lss_.max_item_index();
  FinalizeShrink(lss_.added_series(), shrink_boundary);

  // Act
  const auto new_id = lss_.find_or_emplace(ls2_);
  const auto from_find_after = lss_.find(ls2_);

  // Assert
  EXPECT_EQ(1U, new_id);
  ASSERT_TRUE(from_find_after.has_value());
  EXPECT_EQ(new_id, *from_find_after);
  EXPECT_EQ(ls2_, lss_[new_id]);
}

TEST_F(BimapFiveSeriesFixture, ShrunkStateFindResolvesTailIds) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;
  FinalizeShrink(lss_.added_series(), shrink_boundary);

  // Act
  const auto ls4_id = lss_.find(ls4_);
  const auto ls5_id = lss_.find(ls5_);

  // Assert
  ASSERT_TRUE(ls4_id.has_value());
  ASSERT_TRUE(ls5_id.has_value());
  EXPECT_EQ(shrink_boundary, *ls4_id);
  EXPECT_EQ(shrink_boundary + 1, *ls5_id);
  EXPECT_EQ(ls4_, lss_[*ls4_id]);
  EXPECT_EQ(ls5_, lss_[*ls5_id]);
}

class BimapPartialAddedFixture : public BimapCopierFixture {
 protected:
  LabelViewSet ls1_{{"job", "a"}};
  LabelViewSet ls2_{{"job", "b"}};
  LabelViewSet ls3_{{"job", "c"}};
  LabelViewSet ls4_{{"job", "d"}};
  LabelViewSet ls5_{{"job", "e"}};
  Lss seeded_lss_;
  Lss lss_copy_;

  void SetUp() override {
    BimapCopierFixture::SetUp();
    seeded_lss_.find_or_emplace(ls1_);
    seeded_lss_.find_or_emplace(ls2_);
    seeded_lss_.find_or_emplace(ls3_);
    seeded_lss_.find_or_emplace(ls4_);
    seeded_lss_.find_or_emplace(ls5_);
    seeded_lss_.build_deferred_indexes();

    dst_src_ids_mapping_.clear();
    Copier copier(seeded_lss_, seeded_lss_.sorting_index(), seeded_lss_.added_series(), lss_, dst_src_ids_mapping_);
    copier.copy_added_series_and_build_indexes();

    // Keep ids >= shrink boundary alive and preserve one pre-boundary series.
    [[maybe_unused]] const auto touched_ls2 = lss_.find_or_emplace(ls2_);  // id 1
    [[maybe_unused]] const auto touched_ls4 = lss_.find_or_emplace(ls4_);  // id 3
    [[maybe_unused]] const auto touched_ls5 = lss_.find_or_emplace(ls5_);  // id 4
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
  std::vector<uint32_t> ids_in_tree_order;
  ids_in_tree_order.reserve(lss_.ls_id_set().size());
  for (const auto id : lss_.ls_id_set()) {
    ids_in_tree_order.emplace_back(static_cast<uint32_t>(id));
  }

  // Assert
  // btree order must stay consistent with current resolve logic in fixed state.
  EXPECT_TRUE(
      std::ranges::is_sorted(ids_in_tree_order, [this](uint32_t lhs, uint32_t rhs) { return std::ranges::lexicographical_compare(lss_[lhs], lss_[rhs]); }));
}

TEST_F(BimapPartialAddedFixture, FixedStateSortingIndexSortKeepsCurrentResolveOrder) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;
  lss_.set_pending_shrink_boundary(shrink_boundary);
  std::vector<uint32_t> ids{4U, 3U, 2U, 1U, 0U};

  // Act
  lss_.sorting_index().sort(ids);

  // Assert
  EXPECT_TRUE(std::ranges::is_sorted(ids, [this](uint32_t lhs, uint32_t rhs) { return std::ranges::lexicographical_compare(lss_[lhs], lss_[rhs]); }));
}

TEST_F(BimapPartialAddedFixture, PartialAddedFindOmitsDeadPreBoundary) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;
  FinalizeShrink(lss_.added_series(), shrink_boundary);

  // Act
  const auto find_ls1 = lss_.find(ls1_);
  const auto find_ls2 = lss_.find(ls2_);
  const auto find_ls3 = lss_.find(ls3_);
  const auto find_ls4 = lss_.find(ls4_);
  const auto find_ls5 = lss_.find(ls5_);

  // Assert
  EXPECT_FALSE(find_ls1.has_value());
  EXPECT_TRUE(find_ls2.has_value());
  EXPECT_EQ(1U, *find_ls2);
  EXPECT_FALSE(find_ls3.has_value());

  ASSERT_TRUE(find_ls4.has_value());
  ASSERT_TRUE(find_ls5.has_value());
  EXPECT_EQ(3U, *find_ls4);
  EXPECT_EQ(4U, *find_ls5);
}

TEST_F(BimapPartialAddedFixture, PartialAddedResolveEmptiesDeadSlots) {
  // Arrange
  constexpr uint32_t shrink_boundary = 3U;
  FinalizeShrink(lss_.added_series(), shrink_boundary);

  // Act
  const auto ls0 = lss_[0];
  const auto ls1 = lss_[1];
  const auto ls2 = lss_[2];
  const auto ls3 = lss_[3];
  const auto ls4 = lss_[4];

  // Assert
  EXPECT_EQ(0U, lss_[0].size());
  EXPECT_EQ(ls2_, ls1);
  EXPECT_EQ(0U, ls2.size());
  EXPECT_EQ(ls4_, ls3);
  EXPECT_EQ(ls5_, ls4);
  EXPECT_EQ(0U, ls0.size());
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
  const auto recreated_ls1 = lss_.find_or_emplace(ls1_);
  const auto recreated_ls3 = lss_.find_or_emplace(ls3_);
  const auto find_ls1 = lss_.find(ls1_);
  const auto find_ls3 = lss_.find(ls3_);

  // Assert
  EXPECT_GE(recreated_ls1, shrink_boundary);
  EXPECT_GE(recreated_ls3, shrink_boundary);
  ASSERT_TRUE(find_ls1.has_value());
  ASSERT_TRUE(find_ls3.has_value());
  EXPECT_EQ(recreated_ls1, *find_ls1);
  EXPECT_EQ(recreated_ls3, *find_ls3);
}

class BimapShrinkTwoFixture : public BimapCopierFixture {
 protected:
  LabelViewSet ls1_{{"job", "a"}};
  LabelViewSet ls2_{{"job", "b"}};
  Lss lss_copy_;

  void SetUp() override {
    BimapCopierFixture::SetUp();
    lss_.find_or_emplace(ls1_);
    lss_.find_or_emplace(ls2_);
    lss_.build_deferred_indexes();
  }

  void RunFinalizeShrinkWithSnapshot(const BareBones::Vector<uint32_t>& ids_for_copy) {
    const uint32_t shrink_boundary = lss_.max_item_index();
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
  EXPECT_TRUE(std::ranges::equal(ls1_, lss_[0]));
  EXPECT_TRUE(std::ranges::equal(ls2_, lss_[1]));
}

TEST_F(BimapShrinkTwoFixture, FinalizeShrinkSnapshotKeepsFindWorking) {
  // Arrange
  RunFinalizeShrinkWithSnapshot(BareBones::Vector<uint32_t>{0U, 1U});

  // Act
  const auto from_find_first = lss_.find(ls1_);
  const auto from_find_second = lss_.find(ls2_);

  // Assert
  ASSERT_TRUE(from_find_first.has_value());
  ASSERT_TRUE(from_find_second.has_value());
  EXPECT_EQ(0U, *from_find_first);
  EXPECT_EQ(1U, *from_find_second);
  EXPECT_TRUE(std::ranges::equal(ls1_, lss_[*from_find_first]));
  EXPECT_TRUE(std::ranges::equal(ls2_, lss_[*from_find_second]));
}

TEST_F(BimapShrinkTwoFixture, ShrunkTwoSeriesCountMatchesIndices) {
  // Arrange

  // Act
  RunFinalizeShrinkWithSnapshot(BareBones::Vector<uint32_t>{0U, 1U});

  // Assert
  EXPECT_EQ(2U, lss_.series_count());
  EXPECT_EQ(2U, lss_.max_item_index());
}

}  // namespace
