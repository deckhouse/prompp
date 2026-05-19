#include <gtest/gtest.h>

#include <sstream>
#include <string>

#include "primitives/label_set.h"
#include "primitives/snug_composites.h"
#include "series_index/prometheus/tsdb/index/index_writer.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using series_index::QueryableEncodingBimapCopier;
using series_index::prometheus::tsdb::index::IndexWriter;

template <class DecodingTable, class SortingIndex, class SeriesIds, class QueryableEncodingBimap, class LsIdVector>
using Copier = QueryableEncodingBimapCopier<DecodingTable, SortingIndex, SeriesIds, QueryableEncodingBimap, LsIdVector>;

class IndexWriterShrunkFixture : public testing::Test {
 protected:
  using Lss = series_index::QueryableEncodingBimap<BareBones::Vector>;
  using Stream = std::ostringstream;
  using Writer = IndexWriter<Lss, Stream>;

  Lss normal_lss_;
  Lss shrunk_lss_;
  Lss snapshot_copy_;

  void SetUp() override {
    normal_lss_.find_or_emplace(LabelViewSet{{"job", "cron"}, {"server", "localhost"}});
    normal_lss_.find_or_emplace(LabelViewSet{{"job", "cron"}, {"server", "127.0.0.1"}});
    normal_lss_.build_deferred_indexes();

    shrunk_lss_.find_or_emplace(LabelViewSet{{"job", "cron"}, {"server", "localhost"}});
    shrunk_lss_.find_or_emplace(LabelViewSet{{"job", "cron"}, {"server", "127.0.0.1"}});
    shrunk_lss_.build_deferred_indexes();
    finalize_shrink_all_into_snapshot();
  }

  void finalize_shrink_all_into_snapshot() {
    const uint32_t shrink_boundary = shrunk_lss_.next_item_index();
    BareBones::Vector<uint32_t> dst_src_ids_mapping;
    Copier<Lss, decltype(shrunk_lss_.sorting_index()), decltype(shrunk_lss_.added_series()), Lss, BareBones::Vector<uint32_t>> copier(
        shrunk_lss_, shrunk_lss_.sorting_index(), shrunk_lss_.added_series(), snapshot_copy_, dst_src_ids_mapping);
    copier.copy_added_series_and_build_indexes();

    shrunk_lss_.set_pending_shrink_boundary(shrink_boundary);
    shrunk_lss_.finalize_copy_and_shrink(snapshot_copy_, dst_src_ids_mapping);
  }

  static std::string write_index(const Lss& lss) {
    Stream stream;
    Writer writer{lss};
    writer.write(stream);

    return stream.str();
  }
};

TEST_F(IndexWriterShrunkFixture, WritesSameIndexForNormalAndShrunkAllFromSnapshot) {
  // Arrange
  const auto normal = write_index(normal_lss_);

  // Act
  const auto shrunk = write_index(shrunk_lss_);

  // Assert
  EXPECT_EQ(normal, shrunk);
}

TEST_F(IndexWriterShrunkFixture, WritesSameIndexForNormalAndShrunkWithSeriesAddedAfterShrink) {
  // Arrange
  const LabelViewSet added_after_shrink{{"job", "cron"}, {"server", "remote"}};
  const auto normal_id = normal_lss_.find_or_emplace(added_after_shrink);
  const auto shrunk_id = shrunk_lss_.find_or_emplace(added_after_shrink);
  const auto normal = write_index(normal_lss_);

  // Act
  const auto shrunk = write_index(shrunk_lss_);

  // Assert
  EXPECT_EQ(2U, normal_id);
  EXPECT_EQ(normal_id, shrunk_id);
  EXPECT_EQ(normal, shrunk);
}

}  // namespace
