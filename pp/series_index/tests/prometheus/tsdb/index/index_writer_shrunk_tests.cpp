#include <gtest/gtest.h>

#include <limits>
#include <memory>
#include <sstream>
#include <string>
#include <vector>

#include "primitives/label_set.h"
#include "primitives/snug_composites.h"
#include "series_index/prometheus/tsdb/index/index_writer.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using series_index::invert_copy_mapping;
using series_index::QueryableEncodingBimapCopier;
using series_index::prometheus::tsdb::index::ChunkMetadata;
using series_index::prometheus::tsdb::index::IndexWriter;

template <class DecodingTable, class SortingIndex, class SeriesIds, class QueryableEncodingBimap, class LsIdVector>
using Copier = QueryableEncodingBimapCopier<DecodingTable, SortingIndex, SeriesIds, QueryableEncodingBimap, LsIdVector>;

class IndexWriterShrunkFixture : public testing::Test {
 protected:
  using Lss = series_index::QueryableEncodingBimap<BareBones::Vector>;
  using Stream = std::ostringstream;
  using Writer = IndexWriter<Lss, Stream>;
  using ChunkMetadataList = std::vector<ChunkMetadata>;

  Lss normal_lss_;
  Lss shrunk_lss_;
  std::unique_ptr<Lss> snapshot_copy_;
  BareBones::Vector<uint32_t> old_to_new_;

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
    snapshot_copy_ = std::make_unique<Lss>();
    BareBones::Vector<uint32_t> dst_src_ids_mapping;
    Copier<Lss, decltype(shrunk_lss_.sorting_index()), decltype(shrunk_lss_.added_series()), Lss, BareBones::Vector<uint32_t>> copier(
        shrunk_lss_, shrunk_lss_.sorting_index(), shrunk_lss_.added_series(), *snapshot_copy_, dst_src_ids_mapping);
    copier.copy_added_series_and_build_indexes();

    invert_copy_mapping(dst_src_ids_mapping, shrink_boundary, old_to_new_);
    shrunk_lss_.set_pending_shrink_boundary(shrink_boundary);
    shrunk_lss_.finalize_copy_and_shrink(*snapshot_copy_, old_to_new_);
  }

  static std::string write_index(const Lss& lss) {
    Stream stream;
    Writer writer{lss};

    writer.write_header(stream);
    writer.write_symbols(stream);
    for (uint32_t ls_id = 0; ls_id < lss.next_item_index(); ++ls_id) {
      if (lss[ls_id].size() == 0) {
        continue;
      }
      writer.write_series(ls_id, ChunkMetadataList{}, stream);
    }
    while (writer.has_more_postings_data()) {
      writer.write_postings(stream, std::numeric_limits<uint32_t>::max());
    }
    writer.write_label_indices(stream);
    writer.write_label_indices_table(stream);
    writer.write_postings_table_offsets(stream);
    writer.write_toc(stream);

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

}  // namespace
