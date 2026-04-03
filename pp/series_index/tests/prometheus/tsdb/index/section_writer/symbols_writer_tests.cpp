#include <gtest/gtest.h>

#include "primitives/label_set.h"
#include "primitives/snug_composites.h"
#include "series_index/prometheus/tsdb/index/index_write_context.h"
#include "series_index/prometheus/tsdb/index/section_writer/symbols_writer.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using PromPP::Prometheus::tsdb::index::StreamWriter;
using series_index::QueryableEncodingBimapCopier;
using series_index::SeriesReverseIndex;
using series_index::prometheus::tsdb::index::section_writer::SymbolsWriter;
using std::operator""sv;

template <class DecodingTable, class SortingIndex, class SeriesIds, class QueryableEncodingBimap, class LsIdVector>
using Copier = QueryableEncodingBimapCopier<DecodingTable, SortingIndex, SeriesIds, QueryableEncodingBimap, LsIdVector>;

struct SymbolsWriterCase {
  std::vector<LabelViewSet> label_sets;
  std::string_view expected;
};

LabelViewSet make_ls_with_empty_label_value() {
  LabelViewSet ls{{"key", "value"}};
  for (auto& label : ls) {
    label.second = "";
    break;
  }
  return ls;
}

class SymbolsWriterFixture : public testing::TestWithParam<SymbolsWriterCase> {
 protected:
  using QueryableEncodingBimap = series_index::QueryableEncodingBimap<BareBones::Vector>;

  std::ostringstream stream_;
  StreamWriter<decltype(stream_)> stream_writer_{&stream_};
  QueryableEncodingBimap lss_;

  void SetUp() final {
    for (auto& label_set : GetParam().label_sets) {
      lss_.find_or_emplace(label_set);
    }
  }
};

TEST_P(SymbolsWriterFixture, Test) {
  // Arrange
  const auto index_write_context = series_index::prometheus::tsdb::index::IndexWriteContext<QueryableEncodingBimap>{lss_};

  // Act
  SymbolsWriter<QueryableEncodingBimap, decltype(stream_)> symbols_writer{index_write_context, stream_writer_};
  symbols_writer.write();

  // Assert
  EXPECT_EQ(GetParam().expected, stream_.view());
}

INSTANTIATE_TEST_SUITE_P(EmptyLabelSet,
                         SymbolsWriterFixture,
                         testing::Values(SymbolsWriterCase{.label_sets = {},
                                                           .expected = "\x00\x00\x00\x05"
                                                                       "\x00\x00\x00\x01"
                                                                       "\x0"
                                                                       "\x56\xD0\xEE\x42"sv}));
INSTANTIATE_TEST_SUITE_P(LabelWithEmptyValue,
                         SymbolsWriterFixture,
                         testing::Values(SymbolsWriterCase{.label_sets = {make_ls_with_empty_label_value()},
                                                           .expected = "\x00\x00\x00\x09"
                                                                       "\x00\x00\x00\x02"
                                                                       "\x0"
                                                                       "\x03"
                                                                       "key"
                                                                       "\x22\x8B\x97\x4E"sv}));

INSTANTIATE_TEST_SUITE_P(TestUniquenessAndSorting,
                         SymbolsWriterFixture,
                         testing::Values(SymbolsWriterCase{
                             .label_sets = {{{"job", "cron"}, {"server", "localhost"}}, {{"job", "cron"}, {"server", "127.0.0.1"}}},
                             .expected = "\x00\x00\x00\x29"
                                         "\x00\x00\x00\x06"
                                         "\x00"
                                         "\x09"
                                         "127.0.0.1"
                                         "\x04"
                                         "cron"
                                         "\x03"
                                         "job"
                                         "\x09"
                                         "localhost"
                                         "\x06"
                                         "server"
                                         "\xCB\xE1\x54\x24"sv}));

class SymbolsWriterShrunkLssFixture : public testing::Test {
 protected:
  using Lss = series_index::QueryableEncodingBimap<BareBones::Vector>;
  std::ostringstream stream_;
  StreamWriter<decltype(stream_)> stream_writer_{&stream_};
  Lss lss_;
};

TEST_F(SymbolsWriterShrunkLssFixture, WriteWhenLssShrunkAllFromSnapshot) {
  // Arrange
  lss_.find_or_emplace(LabelViewSet{{"job", "cron"}, {"server", "localhost"}});
  lss_.find_or_emplace(LabelViewSet{{"job", "cron"}, {"server", "127.0.0.1"}});
  lss_.build_deferred_indexes();
  const uint32_t shrink_boundary = lss_.max_item_index();
  Lss lss_copy;
  BareBones::Vector<uint32_t> dst_src_ids_mapping;
  Copier<Lss, decltype(lss_.sorting_index()), decltype(lss_.added_series()), Lss, BareBones::Vector<uint32_t>> copier(
      lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping);
  copier.copy_added_series_and_build_indexes();
  lss_.set_pending_shrink_boundary(shrink_boundary);
  lss_.finalize_copy_and_shrink(lss_copy, dst_src_ids_mapping);

  // Act
  const auto index_write_context = series_index::prometheus::tsdb::index::IndexWriteContext<Lss>{lss_};
  SymbolsWriter<Lss, decltype(stream_)> writer(index_write_context, stream_writer_);
  writer.write();

  // Assert
  EXPECT_EQ(stream_.view(),
            "\x00\x00\x00\x29"
            "\x00\x00\x00\x06"
            "\x00"
            "\x09"
            "127.0.0.1"
            "\x04"
            "cron"
            "\x03"
            "job"
            "\x09"
            "localhost"
            "\x06"
            "server"
            "\xCB\xE1\x54\x24"sv);
}

}  // namespace
