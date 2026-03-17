#include <gmock/gmock-matchers.h>
#include <gtest/gtest.h>

#include "primitives/label_set.h"
#include "primitives/snug_composites.h"
#include "series_index/prometheus/tsdb/index/index_write_context.h"
#include "series_index/queryable_encoding_bimap.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using series_index::invert_copy_mapping;
using series_index::QueryableEncodingBimap;
using series_index::QueryableEncodingBimapCopier;
using series_index::SeriesReverseIndex;

template <class DecodingTable, class SortingIndex, class SeriesIds, class QueryableEncodingBimap, class LsIdVector>
using Copier = QueryableEncodingBimapCopier<DecodingTable, SortingIndex, SeriesIds, QueryableEncodingBimap, LsIdVector>;

class QueryableEncodingBimapFixture : public testing::Test {
 protected:
  using Lss = QueryableEncodingBimap<BareBones::Vector>;

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

TEST_F(QueryableEncodingBimapFixture, Load) {
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

class QueryableEncodingBimapCopierFixture : public QueryableEncodingBimapFixture {
 protected:
  BareBones::Vector<uint32_t> dst_src_ids_mapping_;
};

class QueryableEncodingBimapShrinkFixture : public QueryableEncodingBimapCopierFixture {
 protected:
  LabelViewSet ls1_{{"job", "a"}};
  LabelViewSet ls2_{{"job", "b"}};
  LabelViewSet ls3_{{"job", "c"}};

  void SetUp() override {
    QueryableEncodingBimapCopierFixture::SetUp();
    lss_.find_or_emplace(ls1_);
    lss_.find_or_emplace(ls2_);
    lss_.find_or_emplace(ls3_);
    lss_.build_deferred_indexes();
  }
};

TEST_F(QueryableEncodingBimapCopierFixture, EmptyLss) {
  // Arrange
  Lss lss_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);

  // Act
  copier.copy_added_series_and_build_indexes();

  // Assert
  EXPECT_EQ(0U, lss_copy.size());
  EXPECT_EQ(0U, dst_src_ids_mapping_.size());
}

TEST_F(QueryableEncodingBimapCopierFixture, NonEmptyLss) {
  // Arrange
  const auto label_set = LabelViewSet{{"job", "cron"}, {"key", ""}, {"process", "php1"}};
  const auto label_set2 = LabelViewSet{{"job", "cron"}, {"key", ""}, {"process", "php"}};

  lss_.find_or_emplace(label_set);
  lss_.find_or_emplace(label_set2);

  lss_.build_deferred_indexes();

  Lss lss_copy;
  const BareBones::Vector ids_for_copy{0U, 1U};
  Copier copier(lss_, lss_.sorting_index(), ids_for_copy, lss_copy, dst_src_ids_mapping_);

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

  EXPECT_EQ(ids_for_copy, dst_src_ids_mapping_);
}

TEST_F(QueryableEncodingBimapCopierFixture, NonEmptyLssKeepOrder) {
  // Arrange
  lss_.find_or_emplace(LabelViewSet{{"job", "cron"}, {"key", "1"}, {"process", "php"}});
  lss_.find_or_emplace(LabelViewSet{{"job", "cron"}, {"key", "2"}, {"process", "php"}});
  lss_.find_or_emplace(LabelViewSet{{"job", "cron"}, {"key", "3"}, {"process", "php"}});
  lss_.find_or_emplace(LabelViewSet{{"job", "cron"}, {"key", "4"}, {"process", "php"}});
  lss_.find_or_emplace(LabelViewSet{{"job", "cron"}, {"key", "5"}, {"process", "php"}});

  lss_.build_deferred_indexes();

  Lss lss_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);

  // Act
  copier.copy_added_series_and_build_indexes();

  // Assert
  EXPECT_TRUE(std::ranges::is_sorted(lss_copy.ls_id_set(),
                                     [&](const auto idl, const auto idr) { return std::ranges::lexicographical_compare(lss_copy[idl], lss_copy[idr]); }));
  EXPECT_EQ((BareBones::Vector{0U, 1U, 2U, 3U, 4U}), dst_src_ids_mapping_);
}

TEST_F(QueryableEncodingBimapCopierFixture, SkipSeries) {
  // Arrange
  const auto label_set1 = LabelViewSet{{"job", "cron"}, {"key", "1"}, {"process", "php"}};
  const auto label_set2 = LabelViewSet{{"job", "cron"}, {"key", "2"}, {"process", "php"}};
  const auto label_set3 = LabelViewSet{{"job", "cron"}, {"key", "3"}, {"process", "php"}};
  const auto label_set4 = LabelViewSet{{"job", "cron"}, {"key", "4"}, {"process", "php"}};
  const auto label_set5 = LabelViewSet{{"job", "cron"}, {"key", "5"}, {"process", "php"}};

  lss_.find_or_emplace(label_set1);
  lss_.find_or_emplace(label_set2);
  lss_.find_or_emplace(label_set3);
  lss_.find_or_emplace(label_set4);
  lss_.find_or_emplace(label_set5);

  lss_.build_deferred_indexes();

  Lss lss_copy;
  const BareBones::Vector ids_for_copy{0U, 2U, 4U};
  Copier copier(lss_, lss_.sorting_index(), ids_for_copy, lss_copy, dst_src_ids_mapping_);

  // Act
  copier.copy_added_series_and_build_indexes();

  // Assert
  ASSERT_EQ(3U, lss_copy.size());
  EXPECT_EQ(label_set1, lss_copy[0]);
  EXPECT_EQ(label_set3, lss_copy[1]);
  EXPECT_EQ(label_set5, lss_copy[2]);
  EXPECT_EQ(ids_for_copy, dst_src_ids_mapping_);
}

TEST_F(QueryableEncodingBimapCopierFixture, CopyOfCopy) {
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

  EXPECT_TRUE(dst_src_ids_mapping_.empty());
}

TEST_F(QueryableEncodingBimapCopierFixture, CopyMappingSize) {
  // Arrange
  lss_.find_or_emplace(LabelViewSet{{"job", "a"}});
  lss_.find_or_emplace(LabelViewSet{{"job", "b"}});
  lss_.find_or_emplace(LabelViewSet{{"job", "c"}});
  lss_.find_or_emplace(LabelViewSet{{"job", "d"}});
  lss_.find_or_emplace(LabelViewSet{{"job", "e"}});
  lss_.build_deferred_indexes();

  const uint32_t max_lsid = lss_.next_item_index();
  Lss lss_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);
  copier.copy_added_series_and_build_indexes();

  BareBones::Vector<uint32_t> old_to_new;

  // Act
  invert_copy_mapping(dst_src_ids_mapping_, max_lsid, old_to_new);

  // Assert
  EXPECT_EQ(max_lsid, old_to_new.size());
  EXPECT_EQ(0U, old_to_new[dst_src_ids_mapping_[0]]);
  EXPECT_EQ(1U, old_to_new[dst_src_ids_mapping_[1]]);
  EXPECT_EQ(2U, old_to_new[dst_src_ids_mapping_[2]]);
  EXPECT_EQ(3U, old_to_new[dst_src_ids_mapping_[3]]);
  EXPECT_EQ(4U, old_to_new[dst_src_ids_mapping_[4]]);
}

TEST_F(QueryableEncodingBimapShrinkFixture, ShrinkResolveViaMapping) {
  // Arrange
  const uint32_t shrink_boundary = lss_.next_item_index();
  const uint32_t max_lsid = shrink_boundary;
  Lss lss_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);
  copier.copy_added_series_and_build_indexes();

  BareBones::Vector<uint32_t> old_to_new;
  invert_copy_mapping(dst_src_ids_mapping_, max_lsid, old_to_new);

  // Act
  lss_.fill_touched_series_mapping(shrink_boundary, lss_copy, old_to_new, lss_.added_series());
  lss_.finalize_copy_and_shrink(shrink_boundary, lss_copy, old_to_new);

  // Assert
  EXPECT_EQ(ls1_, lss_[0]);
  EXPECT_EQ(ls2_, lss_[1]);
  EXPECT_EQ(ls3_, lss_[2]);
}

TEST_F(QueryableEncodingBimapCopierFixture, IndicesKeptAfterShrink) {
  // Arrange
  const auto ls1 = LabelViewSet{{"job", "a"}};
  const auto ls2 = LabelViewSet{{"job", "b"}};
  lss_.find_or_emplace(ls1);
  lss_.find_or_emplace(ls2);
  lss_.build_deferred_indexes();

  const uint32_t shrink_boundary = lss_.next_item_index();
  Lss lss_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);
  copier.copy_added_series_and_build_indexes();

  BareBones::Vector<uint32_t> old_to_new;
  invert_copy_mapping(dst_src_ids_mapping_, shrink_boundary, old_to_new);

  // Act
  lss_.fill_touched_series_mapping(shrink_boundary, lss_copy, old_to_new, lss_.added_series());
  lss_.finalize_copy_and_shrink(shrink_boundary, lss_copy, old_to_new);

  // Assert
  EXPECT_TRUE(lss_.trie_index().names_trie().lookup("job"));
  ASSERT_NE(nullptr, lss_.trie_index().values_trie(*lss_.trie_index().names_trie().lookup("job")));
  EXPECT_EQ(ls1, lss_[0]);
  EXPECT_EQ(ls2, lss_[1]);
}

TEST_F(QueryableEncodingBimapShrinkFixture, TouchedSeriesFilledByFinalize) {
  // Arrange
  const uint32_t shrink_boundary = lss_.next_item_index();
  const uint32_t max_lsid = shrink_boundary;
  Lss lss_copy;
  const BareBones::Vector ids_for_copy{0U, 2U};
  Copier copier(lss_, lss_.sorting_index(), ids_for_copy, lss_copy, dst_src_ids_mapping_);
  copier.copy_added_series_and_build_indexes();

  BareBones::Vector<uint32_t> old_to_new;
  invert_copy_mapping(dst_src_ids_mapping_, max_lsid, old_to_new);

  BareBones::Bitset touched;
  touched.resize(max_lsid);
  touched.set(1);

  // Act
  lss_.fill_touched_series_mapping(shrink_boundary, lss_copy, old_to_new, touched);
  lss_.finalize_copy_and_shrink(shrink_boundary, lss_copy, old_to_new);

  // Assert
  EXPECT_EQ(ls1_, lss_[0]);
  EXPECT_EQ(ls2_, lss_[1]);
  EXPECT_EQ(ls3_, lss_[2]);
}

class QueryableEncodingBimapFixedStateFixture : public QueryableEncodingBimapCopierFixture {
 protected:
  LabelViewSet ls1_{{"job", "a"}};
  LabelViewSet ls2_{{"job", "b"}};
  LabelViewSet ls3_{{"job", "c"}};

  void SetUp() override {
    QueryableEncodingBimapCopierFixture::SetUp();
    lss_.find_or_emplace(ls1_);
    lss_.find_or_emplace(ls2_);
    lss_.set_pending_shrink_boundary(3);
    lss_.find_or_emplace(ls3_);
  }
};

TEST_F(QueryableEncodingBimapFixedStateFixture, FindReturnsEmptyForUnmarkedSeries) {
  // Assert
  EXPECT_FALSE(lss_.find(ls3_));
}

TEST_F(QueryableEncodingBimapFixedStateFixture, OperatorBracketReturnsEmptyForUnmarkedSeries) {
  // Assert
  EXPECT_TRUE(std::ranges::equal(ls1_, lss_[0]));
  EXPECT_TRUE(std::ranges::equal(ls2_, lss_[1]));
  EXPECT_EQ(0U, lss_[2].size());
}

class QueryableEncodingBimapShrinkTwoSeriesFixture : public QueryableEncodingBimapCopierFixture {
 protected:
  LabelViewSet ls1_{{"job", "a"}};
  LabelViewSet ls2_{{"job", "b"}};
  Lss lss_copy_;
  BareBones::Vector<uint32_t> old_to_new_;

  void SetUp() override {
    QueryableEncodingBimapCopierFixture::SetUp();
    lss_.find_or_emplace(ls1_);
    lss_.find_or_emplace(ls2_);
    lss_.build_deferred_indexes();
  }

  void RunFinalizeShrinkWithSnapshot(const BareBones::Vector<uint32_t>& ids_for_copy, const BareBones::Bitset& touched) {
    const uint32_t shrink_boundary = lss_.next_item_index();
    Copier copier(lss_, lss_.sorting_index(), ids_for_copy, lss_copy_, dst_src_ids_mapping_);
    copier.copy_added_series_and_build_indexes();
    invert_copy_mapping(dst_src_ids_mapping_, shrink_boundary, old_to_new_);
    lss_.fill_touched_series_mapping(shrink_boundary, lss_copy_, old_to_new_, touched);
    lss_.finalize_copy_and_shrink(shrink_boundary, lss_copy_, old_to_new_);
  }
};

TEST_F(QueryableEncodingBimapShrinkTwoSeriesFixture, FinalizeShrinkWithSnapshot) {
  // Act
  RunFinalizeShrinkWithSnapshot(BareBones::Vector<uint32_t>{0U, 1U}, lss_.added_series());

  // Assert
  EXPECT_TRUE(std::ranges::equal(ls1_, lss_[0]));
  EXPECT_TRUE(std::ranges::equal(ls2_, lss_[1]));
}

TEST_F(QueryableEncodingBimapCopierFixture, EmptyCompositeSizeZero) {
  typename Lss::Base::value_type empty;
  EXPECT_EQ(0U, empty.size());
}

TEST_F(QueryableEncodingBimapShrinkTwoSeriesFixture, ShrinkUnmappedIdReturnsEmpty) {
  // Arrange
  BareBones::Bitset touched;
  touched.resize(2);
  touched.set(0);

  // Act
  RunFinalizeShrinkWithSnapshot(BareBones::Vector<uint32_t>{0U}, touched);

  // Assert
  EXPECT_EQ(0U, lss_[1].size());
  EXPECT_EQ(ls1_, lss_[0]);
}

TEST_F(QueryableEncodingBimapShrinkFixture, IndexWriteContextDeduplicatesSymbolsWhenShrunkAllFromSnapshot) {
  // Arrange
  const uint32_t shrink_boundary = lss_.next_item_index();
  Lss lss_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);
  copier.copy_added_series_and_build_indexes();
  BareBones::Vector<uint32_t> old_to_new;
  invert_copy_mapping(dst_src_ids_mapping_, shrink_boundary, old_to_new);
  lss_.fill_touched_series_mapping(shrink_boundary, lss_copy, old_to_new, lss_.added_series());
  lss_.finalize_copy_and_shrink(shrink_boundary, lss_copy, old_to_new);

  // Act
  const auto ctx = series_index::prometheus::tsdb::index::IndexWriteContext<Lss>{lss_};
  std::vector<std::string> symbols;
  ctx.for_each_symbol([&](uint32_t /*symbol_ref*/, std::string_view s) { symbols.emplace_back(s); });

  // Assert
  EXPECT_THAT(symbols, testing::ElementsAre("", "a", "b", "c", "job"));
}

TEST_F(QueryableEncodingBimapShrinkFixture, IndexWriteContextResolvesSeriesRefsWhenShrunkAllFromSnapshot) {
  // Arrange
  const uint32_t shrink_boundary = lss_.next_item_index();
  Lss lss_copy;
  Copier copier(lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping_);
  copier.copy_added_series_and_build_indexes();
  BareBones::Vector<uint32_t> old_to_new;
  invert_copy_mapping(dst_src_ids_mapping_, shrink_boundary, old_to_new);
  lss_.fill_touched_series_mapping(shrink_boundary, lss_copy, old_to_new, lss_.added_series());
  lss_.finalize_copy_and_shrink(shrink_boundary, lss_copy, old_to_new);
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

}  // namespace
