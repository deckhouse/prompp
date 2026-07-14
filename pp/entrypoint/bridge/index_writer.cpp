#include "index_writer.h"
#include "annotations.h"

#include <memory>

#include "entrypoint/types/lss.h"
#include "primitives/go_slice.h"
#include "series_index/prometheus/tsdb/index/index_writer.h"

namespace {

using PromPP::Primitives::Go::SliceView;
using series_index::prometheus::tsdb::index::ChunkMetadata;
using IndexWriter = series_index::prometheus::tsdb::index::IndexWriter<entrypoint::types::QueryableEncodingBimap, PromPP::Primitives::Go::BytesStream>;

// The writer keeps its output buffer instead of receiving one from Go. Every write_* method
// resets the buffer and fills it, so nothing but the writer pointer crosses the cgo boundary.
// The buffer is freed by its own destructor when the handle is deleted in the writer destructor.
struct IndexWriterHandle {
  IndexWriter writer;
  PromPP::Primitives::Go::Slice<char> buffer;
  // Set after every write_postings batch so Go can decide whether to loop again. Exposed as a
  // stable pointer from the constructor (like the buffer), so reading it needs no extra cgo call.
  uint8_t has_more_postings{0};

  explicit IndexWriterHandle(entrypoint::types::QueryableEncodingBimap& lss) : writer(lss) {}

  PromPP::Primitives::Go::BytesStream reset_buffer() noexcept {
    buffer.resize(0);
    return PromPP::Primitives::Go::BytesStream{&buffer};
  }
};

using IndexWriterHandlePtr = std::unique_ptr<IndexWriterHandle>;

}  // namespace

/**
 * @brief Construct index writer
 *
 * The writer owns an internal output buffer that every write_* method resets
 * and fills, so the buffer is never threaded through the cgo boundary. Besides
 * the writer pointer the constructor returns a stable pointer to that buffer
 * (a Go []byte header: {data, len, cap}); Go reads the produced bytes from it
 * after each call. The buffer is released together with the writer in the
 * destructor.
 *
 * @param args {
 *     lss         uintptr      // pointer to constructed lss
 * }
 * @param res {
 *     writer            uintptr // pointer to constructed index writer
 *     buffer            uintptr // pointer to the writer's internal output buffer ([]byte header)
 *     has_more_postings uintptr // pointer to a uint8 set by write_postings (1 = more batches remain)
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_index_writer_ctor(void* args, void* res) {
  struct Arguments {
    entrypoint::types::LssVariantPtr lss;
  };
  struct Result {
    IndexWriterHandlePtr writer;
    PromPP::Primitives::Go::Slice<char>* buffer;
    uint8_t* has_more_postings;
  };

  const auto in = static_cast<Arguments*>(args);
  auto handle = std::make_unique<IndexWriterHandle>(std::get<entrypoint::types::QueryableEncodingBimap>(*in->lss));
  // Capture the interior pointers before moving the handle into the result (the move nulls it).
  auto* buffer = &handle->buffer;
  auto* has_more_postings = &handle->has_more_postings;
  new (res) Result{.writer = std::move(handle), .buffer = buffer, .has_more_postings = has_more_postings};
}

/**
 * @brief Destroy index writer
 *
 * @param args {
 *     writer    uintptr
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_index_writer_dtor(void* args) {
  struct Arguments {
    IndexWriterHandlePtr writer;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

/**
 * @brief Write header
 *
 * Writes into the writer's internal buffer; read the result from the buffer
 * pointer returned by the constructor.
 *
 * @param writer uintptr // pointer to constructed index writer
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_index_writer_write_header(void* args) {
  using Arguments = IndexWriterHandle;

  auto* handle = static_cast<Arguments*>(args);
  auto stream = handle->reset_buffer();
  handle->writer.write_header(stream);
}

/**
 * @brief Write symbols
 *
 * Long-running single call: invoked as a regular cgo call (not fastcgo) so the
 * goroutine parks in _Gsyscall and frees its P for the duration. The writer
 * pointer is a stable prompp-arena address passed by value, so C runs on its
 * own stack frame and never dereferences a goroutine stack address that a
 * concurrent GC stack move could invalidate. The result is written into the
 * writer's internal buffer.
 *
 * @param writer uintptr // pointer to constructed index writer
 */
extern "C" PROMPP(entrypoint, cgo) void prompp_index_writer_write_symbols(void* writer) {
  auto* handle = static_cast<IndexWriterHandle*>(writer);
  auto stream = handle->reset_buffer();
  handle->writer.write_symbols(stream);
}

/**
 * @brief Write next series batch
 *
 * Writes into the writer's internal buffer; read the result from the buffer
 * pointer returned by the constructor.
 *
 * @param args {
 *     writer      uintptr
 *     chunks_meta []struct{ // chunks metadata slice
 *         min_t     int64
 *         max_t     int64
 *         reference uint64
 *     }
 *     ls_id       uint32
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_index_writer_write_next_series_batch(void* args) {
  struct Arguments {
    IndexWriterHandle* writer;
    SliceView<ChunkMetadata> chunk_metadata_list;
    PromPP::Primitives::LabelSetID ls_id;
  };

  const auto in = static_cast<Arguments*>(args);
  auto stream = in->writer->reset_buffer();
  in->writer->writer.write_series(in->ls_id, in->chunk_metadata_list, stream);
}

/**
 * @brief Write label indices
 *
 * Long-running single call: it walks the whole name/value trie index, so like
 * write_symbols/write_postings it is invoked as a regular cgo call (not fastcgo)
 * to park the goroutine in _Gsyscall and free its P for the duration. The writer
 * pointer is a stable prompp-arena address passed by value. The result is
 * written into the writer's internal buffer.
 *
 * @param writer uintptr // pointer to constructed index writer
 */
extern "C" PROMPP(entrypoint, cgo) void prompp_index_writer_write_label_indices(void* writer) {
  auto* handle = static_cast<IndexWriterHandle*>(writer);
  auto stream = handle->reset_buffer();
  handle->writer.write_label_indices(stream);
}

/**
 * @brief Write one batch of postings
 *
 * Writes postings into the writer's internal buffer until the bytes produced in
 * this call reach max_batch_size, then returns; call repeatedly while the
 * has_more_postings flag (returned by the constructor) is non-zero to drain the
 * whole section. Batching bounds the transient buffer size: a single unbatched
 * call buffers the entire postings section (tens of MiB), so Go flushes each
 * batch and reuses the buffer instead. The byte bound is checked only between
 * whole postings, so the all-series posting and hot label values can overshoot
 * it. Each batch is a regular cgo call (not fastcgo) so the goroutine parks in
 * _Gsyscall and frees its P for the duration; the writer pointer is a stable
 * prompp-arena address passed by value, so no goroutine stack pointer is handed
 * to C.
 *
 * @param writer         uintptr // pointer to constructed index writer
 * @param max_batch_size uint64  // soft upper bound on bytes emitted per call
 */
extern "C" PROMPP(entrypoint, cgo) void prompp_index_writer_write_postings(void* writer, uint64_t max_batch_size) {
  auto* handle = static_cast<IndexWriterHandle*>(writer);
  auto stream = handle->reset_buffer();
  handle->writer.write_postings(stream, static_cast<uint32_t>(max_batch_size));
  handle->has_more_postings = handle->writer.has_more_postings_data() ? 1 : 0;
}

/**
 * @brief Write label indeces table
 *
 * Writes into the writer's internal buffer; read the result from the buffer
 * pointer returned by the constructor.
 *
 * @param writer uintptr // pointer to constructed index writer
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_index_writer_write_label_indices_table(void* args) {
  using Arguments = IndexWriterHandle;

  auto* handle = static_cast<Arguments*>(args);
  auto stream = handle->reset_buffer();
  handle->writer.write_label_indices_table(stream);
}

/**
 * @brief Write postings offset table
 *
 * Writes into the writer's internal buffer; read the result from the buffer
 * pointer returned by the constructor.
 *
 * @param writer uintptr // pointer to constructed index writer
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_index_writer_write_postings_table_offsets(void* args) {
  using Arguments = IndexWriterHandle;

  auto* handle = static_cast<Arguments*>(args);
  auto stream = handle->reset_buffer();
  handle->writer.write_postings_table_offsets(stream);
}

/**
 * @brief Write table of contents
 *
 * Writes into the writer's internal buffer; read the result from the buffer
 * pointer returned by the constructor.
 *
 * @param writer uintptr // pointer to constructed index writer
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_index_writer_write_table_of_contents(void* args) {
  using Arguments = IndexWriterHandle;

  auto* handle = static_cast<Arguments*>(args);
  auto stream = handle->reset_buffer();
  handle->writer.write_toc(stream);
}
