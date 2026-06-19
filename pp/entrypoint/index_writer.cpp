#include "index_writer.h"

#include "head/lss.h"
#include "primitives/go_slice.h"
#include "series_index/prometheus/tsdb/index/index_writer.h"

using PromPP::Primitives::Go::SliceView;
using series_index::prometheus::tsdb::index::ChunkMetadata;
using IndexWriter = series_index::prometheus::tsdb::index::IndexWriter<entrypoint::head::QueryableEncodingBimap, PromPP::Primitives::Go::BytesStream>;

namespace {

// The writer keeps its output buffer instead of receiving one from Go. Every write_* method
// resets the buffer and fills it, so nothing but the writer pointer crosses the cgo boundary.
// The buffer is freed by its own destructor when the handle is deleted in the writer destructor.
struct IndexWriterHandle {
  IndexWriter writer;
  PromPP::Primitives::Go::Slice<char> buffer;

  explicit IndexWriterHandle(entrypoint::head::QueryableEncodingBimap& lss) : writer(lss) {}

  PromPP::Primitives::Go::BytesStream reset_buffer() noexcept {
    buffer.resize(0);
    return PromPP::Primitives::Go::BytesStream{&buffer};
  }
};

}  // namespace

extern "C" void prompp_index_writer_ctor(void* args, void* res) {
  struct Arguments {
    entrypoint::head::LssVariantPtr lss;
  };
  struct Result {
    IndexWriterHandle* writer;
    PromPP::Primitives::Go::Slice<char>* buffer;
  };

  const auto in = static_cast<Arguments*>(args);
  auto* handle = new IndexWriterHandle(std::get<entrypoint::head::QueryableEncodingBimap>(*in->lss));
  *static_cast<Result*>(res) = Result{.writer = handle, .buffer = &handle->buffer};
}

extern "C" void prompp_index_writer_dtor(void* args) {
  struct Arguments {
    IndexWriterHandle* writer;
  };

  delete static_cast<Arguments*>(args)->writer;
}

extern "C" void prompp_index_writer_write_header(void* writer) {
  auto* handle = static_cast<IndexWriterHandle*>(writer);
  auto stream = handle->reset_buffer();
  handle->writer.write_header(stream);
}

extern "C" void prompp_index_writer_write_symbols(void* writer) {
  auto* handle = static_cast<IndexWriterHandle*>(writer);
  auto stream = handle->reset_buffer();
  handle->writer.write_symbols(stream);
}

extern "C" void prompp_index_writer_write_next_series_batch(void* args) {
  struct Arguments {
    IndexWriterHandle* writer;
    SliceView<ChunkMetadata> chunk_metadata_list;
    PromPP::Primitives::LabelSetID ls_id;
  };

  const auto in = static_cast<Arguments*>(args);
  auto stream = in->writer->reset_buffer();
  in->writer->writer.write_series(in->ls_id, in->chunk_metadata_list, stream);
}

extern "C" void prompp_index_writer_write_label_indices(void* writer) {
  auto* handle = static_cast<IndexWriterHandle*>(writer);
  auto stream = handle->reset_buffer();
  handle->writer.write_label_indices(stream);
}

extern "C" void prompp_index_writer_write_postings(void* writer) {
  auto* handle = static_cast<IndexWriterHandle*>(writer);
  auto stream = handle->reset_buffer();
  handle->writer.write_postings(stream);
}

extern "C" void prompp_index_writer_write_label_indices_table(void* writer) {
  auto* handle = static_cast<IndexWriterHandle*>(writer);
  auto stream = handle->reset_buffer();
  handle->writer.write_label_indices_table(stream);
}

extern "C" void prompp_index_writer_write_postings_table_offsets(void* writer) {
  auto* handle = static_cast<IndexWriterHandle*>(writer);
  auto stream = handle->reset_buffer();
  handle->writer.write_postings_table_offsets(stream);
}

extern "C" void prompp_index_writer_write_table_of_contents(void* writer) {
  auto* handle = static_cast<IndexWriterHandle*>(writer);
  auto stream = handle->reset_buffer();
  handle->writer.write_toc(stream);
}
