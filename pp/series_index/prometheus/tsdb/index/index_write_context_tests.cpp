#include <gmock/gmock.h>
#include <gtest/gtest.h>

#include <string>
#include <vector>

#include "bare_bones/vector.h"
#include "primitives/label_set.h"
#include "primitives/snug_composites.h"
#include "series_index/prometheus/tsdb/index/index_write_context.h"
#include "series_index/queryable_encoding_bimap.h"

namespace {

using PromPP::Primitives::LabelViewSet;
template <class T>
using DefaultSharedSpan = BareBones::SharedSpan<T, BareBones::DefaultReallocator>;
using ReadonlyLss = PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<DefaultSharedSpan>;
using series_index::QueryableEncodingBimap;
using series_index::QueryableEncodingBimapCopier;
using series_index::prometheus::tsdb::index::IndexWriteContext;

template <class DecodingTable, class SortingIndex, class SeriesIds, class QueryableEncodingBimap, class LsIdVector>
using Copier = QueryableEncodingBimapCopier<DecodingTable, SortingIndex, SeriesIds, QueryableEncodingBimap, LsIdVector>;

template <class T>
using DefaultSharedVector = BareBones::SharedVector<T, BareBones::DefaultReallocator>;
using Lss = QueryableEncodingBimap<DefaultSharedVector>;

class IndexWriteContextFixture : public testing::Test {
 protected:
  const LabelViewSet ls0_{{"job", "a"}};
  const LabelViewSet ls1_{{"job", "b"}};
  const LabelViewSet ls2_{{"job", "c"}};

  Lss lss_;
  Lss snapshot_copy_;

  void SetUp() override {
    lss_.find_or_emplace(ls0_);
    lss_.find_or_emplace(ls1_);
    lss_.find_or_emplace(ls2_);
    lss_.build_deferred_indexes();
  }

  void FinalizeShrink(const auto& ids_for_copy, uint32_t shrink_boundary) {
    BareBones::Vector<uint32_t> dst_src_ids_mapping;
    Copier copier(lss_, lss_.sorting_index(), ids_for_copy, snapshot_copy_, dst_src_ids_mapping);
    copier.copy_added_series_and_build_indexes();

    lss_.set_pending_shrink_boundary(shrink_boundary);
    const ReadonlyLss resolve_snapshot(snapshot_copy_);
    lss_.finalize_copy_and_shrink(resolve_snapshot, dst_src_ids_mapping);
  }

  [[nodiscard]] std::vector<std::string> CollectSymbols() const {
    const auto context = IndexWriteContext<Lss>{lss_};
    std::vector<std::string> symbols;
    context.for_each_symbol([&](uint32_t, std::string_view symbol) { symbols.emplace_back(symbol); });
    return symbols;
  }
};

TEST_F(IndexWriteContextFixture, DedupesSymbolsAfterFullShrink) {
  // Arrange
  const uint32_t shrink_boundary = lss_.next_item_index();

  // Act
  FinalizeShrink(lss_.added_series(), shrink_boundary);

  // Assert
  EXPECT_THAT(CollectSymbols(), testing::ElementsAre("", "a", "b", "c", "job"));
}

TEST_F(IndexWriteContextFixture, ResolvesRefsAfterFullShrink) {
  // Arrange
  const uint32_t shrink_boundary = lss_.next_item_index();
  FinalizeShrink(lss_.added_series(), shrink_boundary);
  const auto context = IndexWriteContext<Lss>{lss_};

  // Act
  const auto labels0 = lss_[0];
  const auto labels1 = lss_[1];
  const auto labels2 = lss_[2];
  const auto name_ref0 = context.symbol_ref_for_name_for_series(0, labels0.begin().name_id());
  const auto name_ref1 = context.symbol_ref_for_name_for_series(1, labels1.begin().name_id());
  const auto name_ref2 = context.symbol_ref_for_name_for_series(2, labels2.begin().name_id());
  const auto value_ref0 = context.symbol_ref_for_value_for_series(0, labels0.begin().name_id(), labels0.begin().value_id());
  const auto value_ref1 = context.symbol_ref_for_value_for_series(1, labels1.begin().name_id(), labels1.begin().value_id());
  const auto value_ref2 = context.symbol_ref_for_value_for_series(2, labels2.begin().name_id(), labels2.begin().value_id());

  // Assert
  EXPECT_EQ(name_ref0, name_ref1);
  EXPECT_EQ(name_ref1, name_ref2);
  EXPECT_NE(value_ref0, value_ref1);
  EXPECT_NE(value_ref1, value_ref2);
}

TEST_F(IndexWriteContextFixture, ResolvesRefsForSeriesAddedAfterShrink) {
  // Arrange
  const LabelViewSet new_ls{{"job", "d"}};
  const uint32_t shrink_boundary = lss_.next_item_index();
  FinalizeShrink(lss_.added_series(), shrink_boundary);
  const auto new_id = lss_.find_or_emplace(new_ls);
  const auto context = IndexWriteContext<Lss>{lss_};

  // Act
  const auto new_labels = lss_[new_id];
  const auto snapshot_labels = lss_[0];
  const auto new_name_ref = context.symbol_ref_for_name_for_series(new_id, new_labels.begin().name_id());
  const auto snapshot_name_ref = context.symbol_ref_for_name_for_series(0, snapshot_labels.begin().name_id());
  const auto new_value_ref = context.symbol_ref_for_value_for_series(new_id, new_labels.begin().name_id(), new_labels.begin().value_id());
  const auto snapshot_value_ref = context.symbol_ref_for_value_for_series(0, snapshot_labels.begin().name_id(), snapshot_labels.begin().value_id());

  // Assert
  EXPECT_EQ(3U, new_id);
  EXPECT_EQ(new_name_ref, snapshot_name_ref);
  EXPECT_NE(new_value_ref, snapshot_value_ref);
  EXPECT_THAT(CollectSymbols(), testing::ElementsAre("", "a", "b", "c", "d", "job"));
}

// Production rotate: resolve snapshot shares SharedSpans with destination. If destination
// later appends label names using spare SharedVector capacity (no reallocate), a missing
// set_items_count COW inflates snapshot keys past the resolver's frozen symbols_tables_
// and IndexWriteContext::rebuild / for_each_value_id goes OOB (gdb: keys=118, tables=113).
//
// Warm symbol capacity on an EncodingBimap head, checkpoint/rollback so items_count shrinks
// while capacity stays, share into the post-shrink resolver, then grow without reserve().
TEST_F(IndexWriteContextFixture, IndexWriterSurvivesSharedResolveSnapshotSpareCapacityGrowth) {
  using LsBimap = PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<DefaultSharedVector>;

  LsBimap head;
  head.find_or_emplace(ls0_);
  head.find_or_emplace(ls1_);
  head.find_or_emplace(ls2_);

  const auto warmed = head.checkpoint();
  std::vector<std::pair<std::string, std::string>> held_labels;
  held_labels.reserve(80);
  for (uint32_t i = 0; i < 64; ++i) {
    held_labels.emplace_back("cap_" + std::to_string(i), "v");
    head.find_or_emplace(LabelViewSet{{"job", "a"}, {held_labels.back().first, held_labels.back().second}});
  }
  ASSERT_GT(head.data_view().keys().size(), 64U);
  head.rollback(warmed);
  ASSERT_EQ(1U, head.data_view().keys().size());

  Lss copied;
  BareBones::Vector<uint32_t> dst_src_ids_mapping;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), copied, dst_src_ids_mapping);
  copier.copy_added_series_and_build_indexes();

  const uint32_t shrink_boundary = lss_.next_item_index();
  lss_.set_pending_shrink_boundary(shrink_boundary);
  const ReadonlyLss resolve_snapshot(head);
  lss_.finalize_copy_and_shrink(resolve_snapshot, dst_src_ids_mapping);

  const auto keys_at_freeze = resolve_snapshot.data_view().keys().size();
  ASSERT_EQ(1U, keys_at_freeze);
  ASSERT_EQ((*resolve_snapshot.data_view().keys().begin()).data(), (*head.data_view().keys().begin()).data());

  for (uint32_t i = 0; i < 8; ++i) {
    held_labels.emplace_back("post_" + std::to_string(i), "v");
    head.find_or_emplace(LabelViewSet{{"job", "a"}, {held_labels.back().first, held_labels.back().second}});
  }

  const auto snap_keys = resolve_snapshot.data_view().keys().size();
  const auto dest_keys = head.data_view().keys().size();

  ASSERT_GT(dest_keys, keys_at_freeze);
  // Fails without SharedMemory::ensure_unique() in set_items_count (snap keys inflate).
  ASSERT_EQ(keys_at_freeze, snap_keys);

  const auto context = IndexWriteContext<Lss>{lss_};
  std::vector<std::string> symbols;
  context.for_each_symbol([&](uint32_t, std::string_view symbol) { symbols.emplace_back(symbol); });
  EXPECT_FALSE(symbols.empty());
}

}  // namespace
