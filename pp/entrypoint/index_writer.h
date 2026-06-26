#pragma once

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

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
void prompp_index_writer_ctor(void* args, void* res);

/**
 * @brief Destroy index writer
 *
 * @param args {
 *     writer    uintptr
 * }
 */
void prompp_index_writer_dtor(void* args);

/**
 * @brief Write header
 *
 * Writes into the writer's internal buffer; read the result from the buffer
 * pointer returned by the constructor.
 *
 * @param writer uintptr // pointer to constructed index writer
 */
void prompp_index_writer_write_header(void* writer);

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
void prompp_index_writer_write_symbols(void* writer);

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
void prompp_index_writer_write_next_series_batch(void* args);

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
void prompp_index_writer_write_label_indices(void* writer);

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
void prompp_index_writer_write_postings(void* writer, uint64_t max_batch_size);

/**
 * @brief Write label indeces table
 *
 * Writes into the writer's internal buffer; read the result from the buffer
 * pointer returned by the constructor.
 *
 * @param writer uintptr // pointer to constructed index writer
 */
void prompp_index_writer_write_label_indices_table(void* writer);

/**
 * @brief Write postings offset table
 *
 * Writes into the writer's internal buffer; read the result from the buffer
 * pointer returned by the constructor.
 *
 * @param writer uintptr // pointer to constructed index writer
 */
void prompp_index_writer_write_postings_table_offsets(void* writer);

/**
 * @brief Write table of contents
 *
 * Writes into the writer's internal buffer; read the result from the buffer
 * pointer returned by the constructor.
 *
 * @param writer uintptr // pointer to constructed index writer
 */
void prompp_index_writer_write_table_of_contents(void* writer);

#ifdef __cplusplus
}  // extern "C"
#endif
