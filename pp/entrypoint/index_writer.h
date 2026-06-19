#pragma once

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
 *     writer    uintptr        // pointer to constructed index writer
 *     buffer    uintptr        // pointer to the writer's internal output buffer ([]byte header)
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
 * Writes into the writer's internal buffer; read the result from the buffer
 * pointer returned by the constructor.
 *
 * @param writer uintptr // pointer to constructed index writer
 */
void prompp_index_writer_write_label_indices(void* writer);

/**
 * @brief Write all postings in a single call
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
void prompp_index_writer_write_postings(void* writer);

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
