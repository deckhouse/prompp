#include <optional>

#include <gtest/gtest.h>

#include "primitives/label_set.h"
#include "primitives/snug_composites.h"
#include "series_index/prometheus/tsdb/index/index_write_context.h"
#include "series_index/prometheus/tsdb/index/section_writer/label_indices_writer.h"
#include "series_index/prometheus/tsdb/index/section_writer/symbols_writer.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using PromPP::Primitives::LabelViewSet;
using PromPP::Prometheus::tsdb::index::StreamWriter;
using series_index::invert_copy_mapping;
using series_index::QueryableEncodingBimapCopier;
using series_index::SeriesReverseIndex;
using series_index::prometheus::tsdb::index::section_writer::LabelIndicesWriter;
using series_index::prometheus::tsdb::index::section_writer::SymbolsWriter;
using std::operator""sv;

template <class DecodingTable, class SortingIndex, class SeriesIds, class QueryableEncodingBimap, class LsIdVector>
using Copier = QueryableEncodingBimapCopier<DecodingTable, SortingIndex, SeriesIds, QueryableEncodingBimap, LsIdVector>;

struct LabelIndicesWriterCase {
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

class LabelIndicesWriterFixture : public testing::TestWithParam<LabelIndicesWriterCase> {
 protected:
  using QueryableEncodingBimap = series_index::QueryableEncodingBimap<BareBones::Vector>;

  std::ostringstream stream_;
  StreamWriter<decltype(stream_)> stream_writer_{&stream_};
  QueryableEncodingBimap lss_;
  std::optional<series_index::prometheus::tsdb::index::IndexWriteContext<QueryableEncodingBimap>> index_write_context_;
  LabelIndicesWriter<QueryableEncodingBimap, decltype(stream_)> label_indices_writer{lss_, stream_writer_};

  void SetUp() final {
    for (auto& label_set : GetParam().label_sets) {
      lss_.find_or_emplace(label_set);
    }

    std::ostringstream stream;
    StreamWriter<decltype(stream_)> stream_writer{&stream};
    index_write_context_.emplace(lss_);
    label_indices_writer.set_index_write_context(&*index_write_context_);
    SymbolsWriter<QueryableEncodingBimap, decltype(stream_)>{*index_write_context_, stream_writer}.write();
  }
};

TEST_P(LabelIndicesWriterFixture, Test) {
  // Arrange

  // Act
  label_indices_writer.write_label_indices();
  label_indices_writer.write_label_indices_table();

  // Assert
  EXPECT_EQ(GetParam().expected, stream_.view());
}

INSTANTIATE_TEST_SUITE_P(EmptyLabelSet,
                         LabelIndicesWriterFixture,
                         testing::Values(LabelIndicesWriterCase{.label_sets = {},
                                                                .expected = "\x00\x00\x00\x04"
                                                                            "\x00\x00\x00\x00"
                                                                            "\x48\x67\x4B\xC7"sv}));

INSTANTIATE_TEST_SUITE_P(LabelWithEmptyValue,
                         LabelIndicesWriterFixture,
                         testing::Values(LabelIndicesWriterCase{.label_sets = {make_ls_with_empty_label_value()},
                                                                .expected = "\x00\x00\x00\x04"
                                                                            "\x00\x00\x00\x00"
                                                                            "\x48\x67\x4B\xC7"sv}));

INSTANTIATE_TEST_SUITE_P(Test,
                         LabelIndicesWriterFixture,
                         testing::Values(LabelIndicesWriterCase{
                             .label_sets = {{{"job", "cron"}, {"server", "localhost"}}, {{"job", "cron"}, {"server", "127.0.0.1"}}},
                             .expected = "\x00\x00\x00\x0C"
                                         "\x00\x00\x00\x01"
                                         "\x00\x00\x00\x01"
                                         "\x00\x00\x00\x02"
                                         "\x06\x74\x7C\x4E"

                                         "\x00\x00\x00\x10"
                                         "\x00\x00\x00\x01"
                                         "\x00\x00\x00\x02"
                                         "\x00\x00\x00\x01"
                                         "\x00\x00\x00\x04"
                                         "\x60\xB8\x80\x5D"

                                         "\x00\x00\x00\x13"
                                         "\x00\x00\x00\x02"
                                         "\x01"
                                         "\x03"
                                         "job"
                                         "\x00"

                                         "\x01"
                                         "\x06"
                                         "server"
                                         "\x14"
                                         "\xA8\xE9\x05\xC6"sv}));

class LabelIndicesWriterShrunkLssFixture : public testing::Test {
 protected:
  using Lss = series_index::QueryableEncodingBimap<BareBones::Vector>;

  std::ostringstream stream_;
  StreamWriter<decltype(stream_)> stream_writer_{&stream_};
  Lss lss_;
};

TEST_F(LabelIndicesWriterShrunkLssFixture, WriteWhenLssShrunkAllFromSnapshot) {
  // Arrange
  lss_.find_or_emplace(LabelViewSet{{"job", "cron"}, {"server", "localhost"}});
  lss_.find_or_emplace(LabelViewSet{{"job", "cron"}, {"server", "127.0.0.1"}});
  lss_.build_deferred_indexes();
  const uint32_t shrink_boundary = lss_.next_item_index();
  Lss lss_copy;
  BareBones::Vector<uint32_t> dst_src_ids_mapping;
  Copier<Lss, decltype(lss_.sorting_index()), decltype(lss_.added_series()), Lss, BareBones::Vector<uint32_t>> copier(
      lss_, lss_.sorting_index(), lss_.added_series(), lss_copy, dst_src_ids_mapping);
  copier.copy_added_series_and_build_indexes();
  BareBones::Vector<uint32_t> old_to_new;
  invert_copy_mapping(dst_src_ids_mapping, shrink_boundary, old_to_new);
  lss_.fill_touched_series_mapping(shrink_boundary, lss_copy, old_to_new, lss_.added_series());
  lss_.finalize_copy_and_shrink(shrink_boundary, lss_copy, old_to_new);
  const auto index_write_context = series_index::prometheus::tsdb::index::IndexWriteContext<Lss>{lss_};

  std::ostringstream symbols_stream;
  StreamWriter<decltype(symbols_stream)> symbols_stream_writer{&symbols_stream};
  SymbolsWriter<Lss, decltype(symbols_stream)> symbols_writer{index_write_context, symbols_stream_writer};
  symbols_writer.write();

  LabelIndicesWriter<Lss, decltype(stream_)> label_indices_writer{lss_, stream_writer_};
  label_indices_writer.set_index_write_context(&index_write_context);

  // Act
  label_indices_writer.write_label_indices();
  label_indices_writer.write_label_indices_table();

  // Assert
  EXPECT_EQ(stream_.view(),
            "\x00\x00\x00\x0C"
            "\x00\x00\x00\x01"
            "\x00\x00\x00\x01"
            "\x00\x00\x00\x02"
            "\x06\x74\x7C\x4E"
            "\x00\x00\x00\x10"
            "\x00\x00\x00\x01"
            "\x00\x00\x00\x02"
            "\x00\x00\x00\x01"
            "\x00\x00\x00\x04"
            "\x60\xB8\x80\x5D"
            "\x00\x00\x00\x13"
            "\x00\x00\x00\x02"
            "\x01"
            "\x03"
            "job"
            "\x00"
            "\x01"
            "\x06"
            "server"
            "\x14"
            "\xA8\xE9\x05\xC6"sv);
}

}  // namespace
