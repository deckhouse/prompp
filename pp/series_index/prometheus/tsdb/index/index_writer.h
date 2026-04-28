#pragma once

#include "prometheus/tsdb/index/toc_writer.h"
#include "section_writer/label_indices_writer.h"
#include "section_writer/postings_writer.h"
#include "section_writer/series_writer.h"
#include "section_writer/symbols_writer.h"
#include "series_index/prometheus/tsdb/index/index_write_context.h"
#include "types.h"

namespace series_index::prometheus::tsdb::index {

template <class QueryableEncodingBimap, class Stream>
class IndexWriter {
 public:
  using StreamWriter = PromPP::Prometheus::tsdb::index::StreamWriter<Stream>;
  using SeriesWriter = section_writer::SeriesWriter<QueryableEncodingBimap, Stream>;
  using PostingsWriter = section_writer::PostingsWriter<QueryableEncodingBimap, Stream>;
  using LabelIndicesWriter = section_writer::LabelIndicesWriter<QueryableEncodingBimap, Stream>;
  using ExportContext = series_index::prometheus::tsdb::index::IndexWriteContext<QueryableEncodingBimap>;

  explicit IndexWriter(const QueryableEncodingBimap& lss) : lss_(lss) {}

  PROMPP_ALWAYS_INLINE void write_header(Stream& stream) {
    const auto stream_setter = writer_.writer().stream_setter(&stream);

    writer_.write_uint32(PromPP::Prometheus::tsdb::index::kMagic);
    writer_.write(PromPP::Prometheus::tsdb::index::kFormatVersion);
  }

  PROMPP_ALWAYS_INLINE void write_symbols(Stream& stream) {
    const auto stream_setter = writer_.writer().stream_setter(&stream);

    toc_.symbols = writer_.position();
    index_write_context_.rebuild();
    label_indices_writer_.set_index_write_context(&index_write_context_);
    section_writer::SymbolsWriter<QueryableEncodingBimap, Stream>{index_write_context_, writer_}.write();
  }

  template <class ChunkMetadataContainer>
  PROMPP_ALWAYS_INLINE void write_series(PromPP::Primitives::LabelSetID ls_id, const ChunkMetadataContainer& chunks, Stream& stream) {
    const auto stream_setter = writer_.writer().stream_setter(&stream);

    if (toc_.series == 0) [[unlikely]] {
      toc_.series = writer_.position();
    }
    section_writer::SeriesWriter<QueryableEncodingBimap, Stream>{lss_, index_write_context_, series_references_}.write(ls_id, chunks, writer_);
  }

  PROMPP_ALWAYS_INLINE void write_label_indices(Stream& stream) {
    const auto stream_setter = writer_.writer().stream_setter(&stream);

    toc_.label_indices = writer_.position();
    label_indices_writer_.write_label_indices();
  }

  PROMPP_ALWAYS_INLINE void write_postings(Stream& stream, uint32_t max_batch_size) {
    const auto stream_setter = writer_.writer().stream_setter(&stream);

    if (toc_.postings == 0) [[unlikely]] {
      toc_.postings = writer_.position();
    }
    postings_writer_.write_postings(max_batch_size);
  }

  PROMPP_ALWAYS_INLINE void write_label_indices_table(Stream& stream) {
    const auto stream_setter = writer_.writer().stream_setter(&stream);

    toc_.label_indices_table = writer_.position();
    label_indices_writer_.write_label_indices_table();
  }

  PROMPP_ALWAYS_INLINE void write_postings_table_offsets(Stream& stream) {
    const auto stream_setter = writer_.writer().stream_setter(&stream);

    toc_.postings_offset_table = writer_.position();
    postings_writer_.write_postings_table_offsets();
  }

  PROMPP_ALWAYS_INLINE void write_toc(Stream& stream) {
    const auto stream_setter = writer_.writer().stream_setter(&stream);

    PromPP::Prometheus::tsdb::index::TocWriter{toc_, writer_}.write();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool has_more_postings_data() const noexcept { return postings_writer_.has_more_data(); }

 private:
  const QueryableEncodingBimap& lss_;

  SeriesReferencesMap series_references_;
  ExportContext index_write_context_{lss_};

  StreamWriter writer_;

  LabelIndicesWriter label_indices_writer_{lss_, writer_};
  PostingsWriter postings_writer_{lss_, series_references_, writer_};

  PromPP::Prometheus::tsdb::index::Toc toc_;
};

}  // namespace series_index::prometheus::tsdb::index