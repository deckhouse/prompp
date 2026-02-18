#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Free memory allocated for response as []byte
 *
 * @param args *[]byte
 */
void prompp_free_bytes(void* args);

/**
 * @brief Return information about using memory by core
 *
 * @param res {
 *   in_use uint64 // bytes in use
 * }
 */
void prompp_mem_info(void* res);

/**
 * @brief Dump jemalloc memory profile to file
 *
 * @param args {
 *     filename string
 * }
 * @param res {
 *     int error
 * }
 */
void prompp_dump_memory_profile(void* args, void* res);

#ifdef __cplusplus
}
#endif
#define Sizeof_SizeT sizeof(size_t)
#define Sizeof_StdVector 24
#define Sizeof_BareBonesVector 16
#define Sizeof_RoaringBitset 40
#define Sizeof_InnerSeries (Sizeof_SizeT + Sizeof_BareBonesVector + Sizeof_RoaringBitset)
#define Sizeof_GoLabels 16

#define Sizeof_SerializedDataIterator 200

#define Sizeof_MetricsIterator 24

#define Sizeof_SegmentSamplesStorage 80
#define Sizeof_RemoteWriteMessageEncoder 32
#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Return head status
 *
 * @param args {
 *     status struct {...} // status returned by prompp_get_head_status
 * }
 *
 */
void prompp_free_head_status(void* args);

/**
 * @brief Return head status from lss.
 *
 * @param args {
 *     lss         uintptr      // pointer to constructed lss
 *     limit       int          // statistics limit
 * }
 *
 * @param res {
 *     status struct {          // head status
 *          label_value_count_by_label_name []struct {
 *              name string
 *              count uint32
 *          }
 *          series_count_by_metric_name []struct {
 *              name string
 *              count uint32
 *          }
 *          memory_in_bytes_by_label_name []struct {
 *              name string
 *              size uint32
 *          }
 *          series_count_by_label_value_pair [] struct {
 *              name string
 *              value string
 *              count uint32
 *          }
 *          num_series      uint32
 *          num_label_pairs uint32
 *     }
 * }
 */
void prompp_get_head_status_lss(void* args, void* res);

/**
 * @brief Return head status from lss.
 *
 * @param args {
 *     dataStorage uintptr      // pointer to constructed data storage
 * }
 *
 * @param res {
 *     status struct {          // head status
 *          time_interval struct {
 *              min int64
 *              max int64
 *          }
 *          chunk_count     uint32
 *     }
 * }
 */
void prompp_get_head_status_data_storage(void* args, void* res);

#ifdef __cplusplus
}  // extern "C"
#endif
#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Construct a new Head WAL encoder
 *
 * @param args {
 *     shardID            uint16  // shard number
 *     logShards          uint8   // logarithm to the base 2 of total shards count
 *.    lss                uintptr // pointer to lss
 * }
 * @param res {
 *     encoder uintptr // pointer to constructed encoder
 * }
 */
void prompp_head_wal_encoder_ctor(void* args, void* res);

/**
 * @brief Create encoder from decoder
 *
 * @param args {
 *     decoder uintptr // pointer to decoder
 * }
 * @param res {
 *     encoder uintptr // pointer to constructed encoder
 * }
 */
void prompp_head_wal_encoder_ctor_from_decoder(void* args, void* res);

/**
 * @brief Destroy Encoder
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 * }
 */
void prompp_head_wal_encoder_dtor(void* args);

/**
 * @brief Add inner series to current segment
 *
 * @param args {
 *     incomingInnerSeries []InnerSeries // go slice with inner series;
 *     encoder  uintptr                  // pointer to constructed encoder;
 * }
 * @param res {
 *     error               []byte         // error string if thrown
 *     samples             uint32         // number of samples in segment
 * }
 */
void prompp_head_wal_encoder_add_inner_series(void* args, void* res);

/**
 * @brief Flush segment
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 * }
 * @param res {
 *     segment            []byte  // segment content
 *     error              []byte  // error string if thrown
 *     samples            uint32  // number of samples in segment
 * }
 */
void prompp_head_wal_encoder_finalize(void* args, void* res);

/**
 * @brief Construct a new Head WAL Decoder
 *
 * @param args {
 *     lss             uintptr // pointer to lss
 *     encoder_version uint8_t // basic encoder version
 * }
 *
 * @param res {
 *     decoder uintptr // pointer to constructed decoder
 * }
 */
void prompp_head_wal_decoder_ctor(void* args, void* res);

/**
 * @brief Destroy decoder
 *
 * @param args {
 *     decoder uintptr // pointer to constructed decoder
 * }
 */
void prompp_head_wal_decoder_dtor(void* args);

/**
 * @brief Decode WAL-segment into protobuf message
 *
 * @param args {
 *     decoder uintptr // pointer to constructed decoder
 *     segment []byte  // segment content
 *    inner_series *InnerSeries // decoded content
 * }
 * @param res {
 *     created_at int64  // timestamp in ns when data was start writed to encoder
 *     encoded_at int64  // timestamp in ns when segment was encoded
 *     samples    uint32 // number of samples in segment
 *     series     uint32 // number of series in segment
 *     segment_id uint32 // processed segment id
 *     earliest_block_sample int64 // min timestamp in block
 *     latest_block_sample int64 // max timestamp in block
 *     error      []byte // error string if thrown
 * }
 */
void prompp_head_wal_decoder_decode(void* args, void* res);

/**
 * @brief Decode WAL-segment into DataStorage
 *
 * @param args {
 *     decoder uintptr // pointer to constructed decoder
 *     segment []byte  // segment content
 *     encoder uintptr // pointer to constructed data_storage encoder
 * }
 * @param res {
 *     createTimestamp int64 // timestamp of earliest sample in wal
 *     encodeTimestamp int64   // timestamp of latest sample in wal
 *     error      []byte // error string if thrown
 * }
 */
void prompp_head_wal_decoder_decode_to_data_storage(void* args, void* res);

#ifdef __cplusplus
}  // extern "C"
#endif
#pragma once

#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Construct index writer
 *
 * @param args {
 *     lss         uintptr      // pointer to constructed lss
 * }
 * @param res {
 *     writer    uintptr
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
 * @param args {
 *     writer    uintptr
 * }
 * @param res {
 *     data []byte // only c allocated memory can be re-used
 * }
 */
void prompp_index_writer_write_header(void* args, void* res);

/**
 * @brief Write symbols
 *
 * @param args {
 *     writer    uintptr
 * }
 * @param res {
 *     data []byte // only c allocated memory can be re-used
 * }
 */
void prompp_index_writer_write_symbols(void* args, void* res);

/**
 * @brief Write next series batch
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
 * @param res {
 *     data          []byte // only c allocated memory can be re-used
 * }
 */
void prompp_index_writer_write_next_series_batch(void* args, void* res);

/**
 * @brief Write label indices
 *
 * @param args {
 *     writer    uintptr
 * }
 * @param res {
 *     data []byte // only c allocated memory can be re-used
 * }
 */
void prompp_index_writer_write_label_indices(void* args, void* res);

/**
 * @brief Write next postings batch
 *
 * @param args {
 *     writer         uintptr
 *     max_batch_size uint32
 * }
 * @param res {
 *     data          []byte // only c allocated memory can be re-used
 *     has_more_data bool   // true if we should repeat this call
 * }
 */
void prompp_index_writer_write_next_postings_batch(void* args, void* res);

/**
 * @brief Write label indeces table
 *
 * @param args {
 *     writer    uintptr
 * }
 * @param res {
 *     data []byte // only c allocated memory can be re-used
 * }
 */
void prompp_index_writer_write_label_indices_table(void* args, void* res);

/**
 * @brief Write postings offset table
 *
 * @param args {
 *     writer    uintptr
 * }
 * @param res {
 *     data []byte // only c allocated memory can be re-used
 * }
 */
void prompp_index_writer_write_postings_table_offsets(void* args, void* res);

/**
 * @brief Write table of contents
 *
 * @param args {
 *     writer    uintptr
 * }
 * @param res {
 *     data []byte // only c allocated memory can be re-used
 * }
 */
void prompp_index_writer_write_table_of_contents(void* args, void* res);

#ifdef __cplusplus
}  // extern "C"
#endif
#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief get length label set by series id
 *
 * @param args {
 *     lss    uintptr // pointer to constructed lss;
 *     ls_id  uint32  // series id
 * }
 *
 * @param res {
 *     length int     // length of label set
 * }
 */
void prompp_label_set_length(void* args, void* res);

/**
 * @brief get label set by series id
 *
 * @param args {
 *     lss       uintptr                      // pointer to constructed lss;
 *     ls_id     uint32                       // series id
 * }
 *
 * @param res {
 *     label_set []struct{key, value String}  // label sets
 * }
 */
void prompp_label_set_serialize(void* args, void* res);

/**
 * @brief free label set returned by prompp_label_set_serialize
 *
 * @param args {
 *     label_set []struct{key, value String} // label set
 * }
 */
void prompp_label_set_free(void* args);

/**
 * @brief get size in bytes needed for Bytes method
 *
 * @param args {
 *     lss       uintptr   // pointer to constructed lss;
 *     ls_id     uint32    // series id
 * }
 *
 * @param res {
 *     size uint32
 * }
 */
void prompp_label_set_bytes_size(void* args, void* res);

/**
 * @brief implementation of Bytes method
 *
 * @param args {
 *     lss       uintptr   // pointer to constructed lss;
 *     ls_id     uint32    // series id
 * }
 *
 * @param res {
 *     bytes []byte
 * }
 */
void prompp_label_set_bytes(void* args, void* res);

/**
 * @brief implementation of BytesWithLabels method
 *
 * @param args {
 *     lss       uintptr   // pointer to constructed lss;
 *     ls_id     uint32    // series id
 *     names     []string  // names slice
 * }
 *
 * @param res {
 *     bytes []byte
 * }
 */
void prompp_label_set_bytes_with_labels(void* args, void* res);

/**
 * @brief implementation of BytesWithoutLabels method
 *
 * @param args {
 *     lss       uintptr   // pointer to constructed lss;
 *     ls_id     uint32    // series id
 *     names     []string  // names slice
 * }
 *
 * @param res {
 *     bytes []byte
 * }
 */
void prompp_label_set_bytes_without_labels(void* args, void* res);

#ifdef __cplusplus
}  // extern "C"
#endif
#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Initialize metrics iterator
 *
 * @param args *MetricIterator
 */
void prompp_metrics_iterator_ctor(void* args);

/**
 * @brief Serialize metric into protobuf and advance iterator to next metric
 *
 * @param args {
 *   iterator *MetricIterator // Pointer to constructed iterator
 * }
 *
 * @param res {
 *   metric *cppbridge.CppMetric // Pointer to go metric
 * }
 */
void prompp_metrics_iterator_next(void* args, void* res);

/**
 * @brief Create metrics page for test
 *
 * @param args {
 *   labels []cppbridge.Label  // metric page label set
 *   counterName string        // label name for uint64 counter
 *   counterValue uint64       // value for for uint64 counter
 * }
 *
 * @param res {
 *   page uintptr // Pointer to constructed page
 * }
 */
void prompp_metrics_page_for_test_ctor(void* args, void* res);

/**
 * @brief Detach metrics page from storage
 *
 * @param args {
 *   page uintptr // Pointer to constructed page
 * }
 */
void prompp_metrics_page_for_test_detach(void* args);

#ifdef __cplusplus
}
#endif
#ifdef __cplusplus
extern "C" {
#endif

#include <stdint.h>

/**
 * @brief Construct a new Primitives label sets.
 *
 * @param args {
 *     lss_type uint32 // type of lss;
 * }
 *
 * @param res {
 *     lss uintptr     // pointer to constructed label sets;
 * }
 */
void prompp_primitives_lss_ctor(void* args, void* res);

/**
 * @brief Destroy Primitives label sets.
 *
 * @param args {
 *     lss uintptr // pointer of label sets;
 * }
 */
void prompp_primitives_lss_dtor(void* args);

/**
 * @brief return size of allocated memory for label sets.
 *
 * @param args {
 *     lss uintptr             // pointer to constructed label sets;
 * }
 *
 * @param res {
 *     allocated_memory uint64 // size of allocated memory for label sets;
 * }
 */
void prompp_primitives_lss_allocated_memory(void* args, void* res);

/**
 * @brief insert label set into lss
 *
 * @param args {
 *     lss uintptr              // pointer to constructed lss;
 *     label_set model.LabelSet // label set
 * }
 *
 * @param res {
 *     ls_id uint32                  // inserted (or found) label set id
 *     bool  lss_has_reallocations   // true if lss has reallocations
 * }
 */
void prompp_primitives_lss_find_or_emplace(void* args, void* res);

/**
 * @brief insert label set builder into lss
 *
 * @param args {
 *     lss uintptr                    // pointer to constructed lss;
 *     builder struct {
 *        readonly_lss uintptr        // pointer to constructed lss;
 *        ls_id        uint32         // series id
 *        sorted_add   []model.Label  // slice of sorted by name labels
 *        sorted_del   []string       // slice of sorted label names
 *     }
 * }
 *
 * @param res {
 *     ls_id uint32                   // inserted (or found) label set id
 *     bool  lss_has_reallocations    // true if lss has reallocations
 * }
 */
void prompp_primitives_lss_find_or_emplace_builder(void* args, void* res);

/**
 * @brief query selector from lss for label matchers
 *
 * @param args {
 *     lss uintptr                         // pointer to constructed queryable lss;
 *     label_matchers []model.LabelMatcher // label matchers
 * }
 *
 * @param res {
 *     selector uintptr // constructed selector
 *     status   uint32  // query status
 * }
 */
void prompp_primitives_lss_query_selector(void* args, void* res);

/**
 * @brief query selector from lss for label matchers
 *
 * @param args {
 *     lss uintptr // pointer to readonly lss
 *     selector uintptr // pointer to constructed selector
 * }
 *
 * @param res {
 *     matches           []uint32 // matched series ids
 *     label_set_lengths []uint16 // slice of series label set length
 *     status            uint32   // query status
 * }
 */
void prompp_primitives_lss_query(void* args, void* res);

/**
 * @brief free label set matches returned by prompp_primitives_lss_query
 *
 * @param args {
 *     matches           []uint32 // matched series ids
 *     label_set_lengths []uint16 // slice of series label set length
 *     status            uint32   // query status
 * }
 */
void prompp_primitives_lss_query_result_free(void* args);

/**
 * @brief get label sets by series id
 *
 * @param args {
 *     lss uintptr    // pointer to constructed lss;
 *     ls_id []uint32 // series ids
 * }
 *
 * @param res {
 *     label_sets [][]struct {key, value String} // label sets
 * }
 */
void prompp_primitives_lss_get_label_sets(void* args, void* res);

/**
 * @brief free label sets returned by prompp_primitives_lss_get_label_sets
 *
 * @param args {
 *     label_sets [][]struct {key, value String} // label set
 * }
 */
void prompp_primitives_lss_free_label_sets(void* args);

/**
 * @brief return size of allocated memory for label sets.
 *
 * @param args {
 *     lss uintptr                         // pointer to constructed queryable lss;
 *     label_matchers []model.LabelMatcher // label matchers
 * }
 *
 * @param res {
 *     status uint32   // query status
 *     names  []string // Slice of string freed by freeBytes in GO pointed to lss memory, so it may be invalid after mutating lss state
 * }
 */
void prompp_primitives_lss_query_label_names(void* args, void* res);

/**
 * @brief return size of allocated memory for label sets.
 *
 * @param args {
 *     lss uintptr                         // pointer to constructed queryable lss;
 *     label_name string                   // label name
 *     label_matchers []model.LabelMatcher // label matchers
 * }
 *
 * @param res {
 *     status uint32   // query status
 *     values []string // Slice of string freed by freeBytes in GO pointed to lss memory, so it may be invalid after mutating lss state
 * }
 */
void prompp_primitives_lss_query_label_values(void* args, void* res);

/**
 * @brief return size of allocated memory for label sets.
 *
 * @param args {
 *     lss uintptr                 // pointer to constructed queryable lss;
 * }
 *
 * @param res {
 *     lss_copy          uintptr  // readonly copy of lss
 * }
 */
void prompp_create_readonly_lss(void* args, void* res);

/**
 * @brief returns a copy of the bitset of added series from the lss.
 *
 * @param args {
 *    lss              uintptr  // pointer to constructed queryable lss;
 * }
 *
 * @param res {
 *     bitset          uintptr  // bitset of added series;
 * }
 */
void prompp_primitives_lss_bitset_series(void* args, void* res);

/**
 * @brief destroy bitset of added series.
 *
 * @param args {
 *     bitset          uintptr  // bitset of added series;
 * }
 *
 */
void prompp_primitives_lss_bitset_dtor(void* args);

/**
 * @brief Copy the label sets from the source lss to the destination lss that were added source lss.
 *
 * @param source_lss pointer to source label sets;
 * @param source_bitset pointer to source bitset;
 * @param destination_lss pointer to destination label sets;
 * @param ids_mapping pointer to uintptr
 *
 * @attention This binding used as a CGO call!!!
 *
 */
void prompp_primitives_readonly_lss_copy_added_series(uint64_t source_lss, uint64_t source_bitset, uint64_t destination_lss, uint64_t ids_mapping);

/**
 * @brief destroy ls ids mapping
 *
 * @param args {
 *     ls_ids_mapping uintptr
 * }
 *
 */
void prompp_primitives_free_ls_ids_mapping(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
#ifdef __cplusplus
extern "C" {
#endif

//
// StatelessRelabeler
//

/**
 * @brief Construct a new StatelessRelabeler.
 *
 * @param args {
 *     cfgs                []*Config // go slice with pointer RelabelConfig;
 * }
 *
 * @param res {
 *     stateless_relabeler uintptr   // pointer to constructed StatelessRelabeler;
 *     error               []byte    // error string if thrown;
 * }
 */
void prompp_prometheus_stateless_relabeler_ctor(void* args, void* res);

/**
 * @brief Destroy StatelessRelabeler
 *
 * @param args {
 *     stateless_relabeler uintptr // pointer of StatelessRelabeler;
 * }
 */
void prompp_prometheus_stateless_relabeler_dtor(void* args);

/**
 * @brief reset_to reset configs and replace on new converting go-config.
 *
 * @param args {
 *     stateless_relabeler uintptr   // pointer to constructed StatelessRelabeler;
 *     cfgs                []*Config // go slice with pointer RelabelConfig;
 * }
 *
 * @param res {
 *     error               []byte    // error string if thrown;
 * }
 */
void prompp_prometheus_stateless_relabeler_reset_to(void* args, void* res);

//
// InnerSeries
//

/**
 * @brief initialize slice of InnerSeries
 *
 * @param args {
 *     innerSeries []InnerSeries
 * }
 */
void prompp_prometheus_inner_series_ctor(void* args);

/**
 * @brief Destroy slice of InnerSeries
 *
 * @param args {
 *      innerSeries []InnerSeries
 * }
 */
void prompp_prometheus_inner_series_dtor(void* args);

/**
 * @brief Reset slice of InnerSeries
 *
 * @param args {
 *      innerSeries []InnerSeries
 * }
 */
void prompp_prometheus_inner_series_reset(void* args);

//
// RelabeledSeries
//

/**
 * @brief initialize slice of RelabeledSeries
 *
 * @param args {
 *     relabeledSeries []RelabeledSeries
 * }
 */
void prompp_prometheus_relabeled_series_ctor(void* args);

/**
 * @brief Destroy slice of RelabeledSeries
 *
 * @param args {
 *      relabeledSeries []RelabeledSeries
 * }
 */
void prompp_prometheus_relabeled_series_dtor(void* args);

/**
 * @brief Reset slice of RelabeledSeries
 *
 * @param args {
 *      relabeledSeries []RelabeledSeries
 * }
 */
void prompp_prometheus_relabeled_series_reset(void* args);

//
// RelabelerStateUpdate
//

/**
 * @brief Initialize slice of RelabelerStateUpdate.
 *
 * @param args {
 *     relabeler_state_update []RelabelerStateUpdate
 * }
 */
void prompp_prometheus_relabeler_state_update_ctor(void* args);

/**
 * @brief Destroy slice of RelabelerStateUpdate.
 *
 * @param args {
 *      relabeledSeries []RelabeledSeries
 * }
 */
void prompp_prometheus_relabeler_state_update_dtor(void* args);

/**
 * @brief Reset slice of RelabelerStateUpdate.
 *
 * @param args {
 *      relabeledSeries []RelabeledSeries
 * }
 */
void prompp_prometheus_relabeler_state_update_reset(void* args);

//
// PerShardRelabeler
//

/**
 * @brief Construct a new PerShardRelabeler.
 *
 * @param args {
 *     external_labels     []Label // slice with external labels;
 *     stateless_relabeler uintptr // pointer to constructed stateless relabeler;
 *     shard_id            uint16  // current shard id;
 *     log_shards          uint8   // logarithm to the base 2 of total shards count;
 * }
 *
 * @param res {
 *     per_shard_relabeler uintptr // pointer to constructed PerShardRelabeler;
 *     error               []byte  // error string if thrown;
 * }
 */
void prompp_prometheus_per_shard_relabeler_ctor(void* args, void* res);

/**
 * @brief Destroy PerShardRelabeler.
 *
 * @param args {
 *     per_shard_relabeler uintptr // pointer of PerShardRelabeler;
 * }
 */
void prompp_prometheus_per_shard_relabeler_dtor(void* args);

/**
 * @brief Create StaleNaNsState.
 *
 * @param res {
 *     state uintptr // pointer to constructed StaleNaNsState;
 * }
 */
void prompp_prometheus_relabel_stale_nans_state_ctor(void* res);

/**
 * @brief Destroy StaleNaNsState.
 *
 * @param args {
 *      state uintptr // pointer to StaleNaNsState;
 * }
 */
void prompp_prometheus_relabel_stale_nans_state_dtor(void* args);

/**
 * @brief add to cache relabled data(third stage).
 *
 * @param args {
 *     relabeler_state_update *RelabelerStateUpdate // pointer to RelabelerStateUpdate;
 *     per_shard_relabeler    uintptr               // pointer to constructed per shard relabeler;
 *     cache                  uintptr               // pointer to constructed Cache;
 *     relabeled_shard_id     uint16                // relabeled shard id;
 * }
 *
 * @param res {
 *     error                  []byte  // error string if thrown;
 * }
 */
void prompp_prometheus_per_shard_single_relabeler_update_relabeler_state(void* args, void* res);

/**
 * @brief relabeling output series(fourth stage).
 *
 * @param args {
 *     incoming_inner_series     []InnerSeries     // go slice with incoming InnerSeries;
 *     encoders_inner_series     []InnerSeries     // go slice with output InnerSeries;
 *     shards_relabeled_series   []*RelabeledSeries // go slice with output RelabeledSeries;
 *     per_shard_relabeler       uintptr            // pointer to constructed per shard relabeler;
 *     lss                       uintptr            // pointer to constructed label sets;
 *     cache                     uintptr            // pointer to constructed Cache;
 * }
 *
 * @param res {
 *     error                   []byte             // error string if thrown;
 * }
 */
void prompp_prometheus_per_shard_relabeler_output_relabeling(void* args, void* res);

/**
 * @brief reset set new number_of_shards and external_labels.
 *
 * @param args {
 *     external_labels     []Label // slice with external lables(pair string);
 *     per_shard_relabeler uintptr // pointer to constructed per shard relabeler;
 *     number_of_shards    uint16  // total shards count;
 * }
 */
void prompp_prometheus_per_shard_relabeler_reset_to(void* args);

//
// Relabeler cache
//

/**
 * @brief Construct a new Cache.
 *
 * @param res {
 *     cache               uintptr // pointer to constructed Cache;
 * }
 */
void prompp_prometheus_cache_ctor(void* res);

/**
 * @brief Destroy Cache.
 *
 * @param args {
 *     cache               uintptr // pointer to constructed Cache;
 * }
 */
void prompp_prometheus_cache_dtor(void* args);

/**
 * @brief return size of allocated memory for caches.
 *
 * @param args {
 *     cache               uintptr // pointer to constructed Cache;
 * }
 *
 * @param res {
 *     allocated_memory    uint64  // size of allocated memory for label sets;
 * }
 */
void prompp_prometheus_cache_allocated_memory(void* args, void* res);

/**
 * @brief add to cache relabled data(third stage).
 *
 * @param args {
 *     shards_relabeler_state_update []*RelabelerStateUpdate // pointer to RelabelerStateUpdate per source shard;
 *     cache                         uintptr                 // pointer to constructed Cache;
 *     relabeled_shard_id            uint16                  // relabeled shard id;
 * }
 *
 * @param res {
 *     error                         []byte                  // error string if thrown;
 * }
 */
void prompp_prometheus_cache_update(void* args, void* res);

//
// PerGoroutineRelabeler
//

/**
 * @brief Construct a new PerGoroutineRelabeler.
 *
 * @param args {
 *     number_of_shards        uint16  // total shards count;
 *     shard_id                uint16  // current shard id;
 * }
 *
 * @param res {
 *     per_goroutine_relabeler uintptr // pointer to constructed PerGoroutineRelabeler;
 * }
 */
void prompp_prometheus_per_goroutine_relabeler_ctor(void* args, void* res);

/**
 * @brief Destroy PerGoroutineRelabeler.
 *
 * @param args {
 *     per_goroutine_relabeler uintptr // pointer of PerGoroutineRelabeler;
 * }
 */
void prompp_prometheus_per_goroutine_relabeler_dtor(void* args);

/**
 * @brief relabeling incomig hashdex(first stage).
 *
 * @param args {
 *     shards_inner_series          []InnerSeries     // go slice with InnerSeries;
 *     shards_relabeled_series      []RelabeledSeries // go slice with RelabeledSeries;
 *     options                      RelabelerOptions   // object RelabelerOptions;
 *     per_goroutine_relabeler      uintptr            // pointer to constructed per goroutine relabeler;
 *     stateless_relabeler          uintptr            // pointer to constructed stateless relabeler;
 *     hashdex                      uintptr            // pointer to filled hashdex;
 *     cache                        uintptr            // pointer to constructed Cache;
 *     input_lss                    uintptr            // pointer to constructed input label sets;
 *     target_lss                   uintptr            // pointer to constructed target label sets;
 * }
 *
 * @param res {
 *     samples_added                uint32             // number of added samples;
 *     series_added                 uint32             // number of added series;
 *     series_drop                  uint32             // number of dropped series;
 *     error                        []byte             // error string if thrown;
 *     target_lss_has_reallocations bool               // true if target lss has reallocations
 * }
 */
void prompp_prometheus_per_goroutine_relabeler_input_relabeling(void* args, void* res);

/**
 * @brief relabeling incoming hashdex(first stage) from cache.
 *
 * @param args {
 *     shards_inner_series     []InnerSeries   // go slice with InnerSeries;
 *     options                 RelabelerOptions // object RelabelerOptions;
 *     per_goroutine_relabeler uintptr          // pointer to constructed per goroutine relabeler;
 *     hashdex                 uintptr          // pointer to filled hashdex;
 *     cache                   uintptr          // pointer to constructed Cache;
 *     input_lss               uintptr          // pointer to constructed input label sets;
 *     target_lss              uintptr          // pointer to constructed target label sets;
 * }
 *
 * @param res {
 *     samples_added       uint32               // number of added samples;
 *     series_added        uint32               // number of added series;
 *     series_drop         uint32               // number of dropped series;
 *     ok                  bool                 // true if all label set find in cache;
 *     error               []byte               // error string if thrown;
 * }
 */
void prompp_prometheus_per_goroutine_relabeler_input_relabeling_from_cache(void* args, void* res);

/**
 * @brief relabeling incoming hashdex(first stage) with state stalenans.
 *
 * @param args {
 *     shards_inner_series          []InnerSeries     // go slice with InnerSeries;
 *     shards_relabeled_series      []RelabeledSeries // go slice with RelabeledSeries;
 *     options                      RelabelerOptions   // object RelabelerOptions;
 *     per_goroutine_relabeler      uintptr            // pointer to constructed per goroutine relabeler;
 *     stateless_relabeler          uintptr            // pointer to constructed stateless relabeler;
 *     hashdex                      uintptr            // pointer to filled hashdex;
 *     cache                        uintptr            // pointer to constructed Cache;
 *     input_lss                    uintptr            // pointer to constructed input label sets;
 *     target_lss                   uintptr            // pointer to constructed target label sets;
 *     def_timestamp                int64              // timestamp for metrics and StaleNaNs
 * }
 *
 * @param res {
 *     samples_added                uint32             // number of added samples;
 *     series_added                 uint32             // number of added series;
 *     series_drop                  uint32             // number of dropped series;
 *     error                        []byte             // error string if thrown;
 *     target_lss_has_reallocations bool               // true if target lss has reallocations
 * }
 */
void prompp_prometheus_per_goroutine_relabeler_input_relabeling_with_stalenans(void* args, void* res);

/**
 * @brief relabeling incomig hashdex(first stage) from cache with state stalenans.
 *
 * @param args {
 *     shards_inner_series     []InnerSeries   // go slice with InnerSeries;
 *     options                 RelabelerOptions // object RelabelerOptions;
 *     per_goroutine_relabeler uintptr          // pointer to constructed per goroutine relabeler;
 *     hashdex                 uintptr          // pointer to filled hashdex;
 *     cache                   uintptr          // pointer to constructed Cache;
 *     input_lss               uintptr          // pointer to constructed input label sets;
 *     target_lss              uintptr          // pointer to constructed target label sets;
 *     def_timestamp           int64            // timestamp for metrics and StaleNaNs
 * }
 *
 * @param res {
 *     samples_added           uint32           // number of added samples;
 *     series_added            uint32           // number of added series;
 *     series_drop             uint32           // number of dropped series;
 *     ok                      bool             // true if all label set find in cache;
 *     error                   []byte           // error string if thrown;
 * }
 */
void prompp_prometheus_per_goroutine_relabeler_input_relabeling_with_stalenans_from_cache(void* args, void* res);

/**
 * @brief transparent relabeling incoming hashdex(first stage).
 *
 * @param args {
 *     shards_inner_series          []InnerSeries     // go slice with InnerSeries;
 *     per_goroutine_relabeler      uintptr            // pointer to constructed per goroutine relabeler;
 *     hashdex                      uintptr            // pointer to filled hashdex;
 *     target_lss                   uintptr            // pointer to constructed target label sets;
 * }
 *
 * @param res {
 *     samples_added                uint32             // number of added samples;
 *     series_added                 uint32             // number of added series;
 *     series_drop                  uint32             // number of dropped series;
 *     error                        []byte             // error string if thrown;
 *     target_lss_has_reallocations bool               // true if target lss has reallocations
 * }
 */
void prompp_prometheus_per_goroutine_relabeler_input_transition_relabeling(void* args, void* res);

/**
 * @brief transparent relabeling incomig hashdex(first stage) from cache.
 *
 * @param args {
 *     shards_inner_series     []InnerSeries   // go slice with InnerSeries;
 *     per_goroutine_relabeler uintptr          // pointer to constructed per goroutine relabeler;
 *     hashdex                 uintptr          // pointer to filled hashdex;
 *     target_lss              uintptr          // pointer to constructed target label sets;
 * }
 *
 * @param res {
 *     samples_added       uint32               // number of added samples;
 *     series_added        uint32               // number of added series;
 *     series_drop         uint32               // number of dropped series;
 *     ok                  bool                 // true if all label set find in cache;
 *     error               []byte               // error string if thrown;
 * }
 */
void prompp_prometheus_per_goroutine_relabeler_input_transition_relabeling_only_read(void* args, void* res);

/**
 * @brief add relabeled ls to lss, add to result and add to cache update(second stage).
 *
 * @param args {
 *     shards_inner_series           []InnerSeries          // go InnerSeries per source shard;
 *     shards_relabeled_series       []RelabeledSeries      // go RelabeledSeries per source shard;
 *     shards_relabeler_state_update []*RelabelerStateUpdate // pointer to RelabelerStateUpdate per source shard;
 *     per_goroutine_relabeler       uintptr                 // pointer to constructed per goroutine relabeler;
 *     target_lss                    uintptr                 // pointer to constructed label sets;
 * }
 *
 * @param res {
 *     error                         []byte                  // error string if thrown
 *     target_lss_has_reallocations  bool                    // true if target lss has reallocations
 * }
 */
void prompp_prometheus_per_goroutine_relabeler_append_relabeler_series(void* args, void* res);

/**
 * @brief add stale nans to inner series if needed
 *
 * @param args {
 *     inner_series      []InnerSeries // InnerSeries
 *     stale_nan_state   uintptr        // pointer to source state
 *     default_timestamp int64          // timestamp for stale_nan samples
 * }
 */
void prompp_prometheus_per_goroutine_relabeler_track_stale_nans(void* args);

/**
 * @brief add stale nans to inner series if needed
 *
 * @param args {
 *     stale_nan_state uintptr  // pointer to source state
 *     ls_ids_mapping  uintptr  // pointer to dst_src_ls_ids_mapping
 * }
 */
void prompp_remap_stale_nans_state(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief create message list
 *
 * @param args {
 *     messagesCount uint64
 * }
 *
 * @param res {
 *     message_list []Message
 * }
 */
void prompp_remote_write_message_list_ctor(void* args, void* res);

/**
 * @brief destroy message list
 *
 * @param args {
 *     message_list []Message
 * }
 */
void prompp_remote_write_message_list_dtor(void* args);

/**
 * @brief create message encoders list
 *
 * @param args {
 *     encodersCount uint64
 * }
 *
 * @param res {
 *     encoders []MessageEncoder
 * }
 */
void prompp_remote_write_message_encoders_ctor(void* args, void* res);

/**
 * @brief destroy message encoders list
 *
 * @param args {
 *     encoders []MessageEncoder
 * }
 */
void prompp_remote_write_message_encoders_dtor(void* args);

/**
 * @brief encode remote write message
 *
 * @param args {
 *     messageEncoder *MessageEncoder
 *     lss_list       []uintptr
 *     storageList    []SegmentSamplesStorageList
 *     messageIndex   uint64
 *     messagesCount  uint64
 *     message        *Message
 * }
 *
 */
void prompp_remote_write_encode_message(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Construct a new series data DataStorage
 *
 * @param res {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 */
void prompp_series_data_data_storage_ctor(void* res);

/**
 * @brief Resets DataStorage to initial state
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 */
void prompp_series_data_data_storage_reset(void* args);

/**
 * @brief Get min max timestamps in storage
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 *
 * @param res {
 *     interval struct {
 *        min int64
 *        max int64
 *     }
 * }
 *
 */
void prompp_series_data_data_storage_time_interval(void* args, void* res);

/**
 * @brief Get queried series bitset memory size
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 *
 * @param res {
 *     size uint32 // queried series bitset memory size
 * }
 *
 */
void prompp_series_data_data_storage_queried_series_bitset_size(void* args, void* res);

/**
 * @brief Get queried series bitset memory
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 *
 * @param res {
 *     queriedSeries []byte // queried series bitset (memory allocated in c++)
 * }
 *
 */
void prompp_series_data_data_storage_queried_series_bitset(void* args, void* res);

/**
 * @brief Get queried series bitset memory
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 *     queriedSeries []byte // queried series bitset memory
 * }
 *
 * @param res {
 *     result bool // load result
 * }
 */
void prompp_series_data_data_storage_queried_series_set_bitset(void* args, void* res);

/**
 * @brief Queries data storage and serializes result.
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 *
 * @param res {
 *     allocated_memory uint64 // serialized data
 * }
 */
void prompp_series_data_data_storage_allocated_memory(void* args, void* res);

/**
 * @brief Queries data storage and serializes result (new serialization model).
 *
 * @param args {
 *     dataStorage    uintptr          // pointer to constructed data storage
 *     query          DataStorageQuery // query
 *     downsamplingMs int64            // downsampling interval in milliseconds (0 - downsampling is disabled)
 * }
 *
 * @param res {
 *     querier uintptr        // pointer to constructed Querier if data loading is needed.
 *                            // If constructed (!= 0) it must be destroyed by calling prompp_series_data_data_storage_query_final.
 *     status  uint8          // status of a query (0 - Success, 1 - Data loading is needed)
 *     serializedData uintptr // pointer to serialized data
 * }
 */
void prompp_series_data_data_storage_query_v2(void* args, void* res);

/**
 * @brief return instant series at given timestamp for label sets.
 *
 * @param args {
 *        dataStorage uintptr      // pointer to constructed data storage
 *        labelSetIDs []uint32     // series ids
 *        timestamp   int64        // timestamp
 *        samples     uintptr      // pointer to samples data
 * }
 * @param res {
 *     InstantQuerier uintptr // pointer to constructed Querier if data loading is needed
 *     Status uint8           // status of a query (0 - Success, 1 - Data loading is needed)
 * }
 */
void prompp_series_data_data_storage_instant_query(void* args, void* res);

/**
 * @brief finishes all Queriers after data load.
 *
 * @param args {
 *        queriers []uintptr    // slice of pointers to Queriers
 *        }
 */
void prompp_series_data_data_storage_query_final(void* args);

/**
 * @brief series data DataStorage destructor.
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 */
void prompp_series_data_data_storage_dtor(void* args);

/**
 * @brief Construct a new ChunkRecoder object for recode all non-empty chunks in dataStorage
 *
 * @param args {
 *     lss uintptr            // pointer to constructed label sets
 *     lsIdBatchSize uint32   // size of ls batch for recoding
 *     dataStorage   uintptr  // pointer to constructed data storage
 *     timeInterval struct {  // closed interval [min, max]
 *        min int64
 *        max int64
 *     }
 *     downsamplingMs int64   // downsampling interval in milliseconds (0 - downsampling is disabled)
 * }
 * @param res {
 *     chunk_recoder uintptr // pointer to chunk recoder
 * }
 */
void prompp_series_data_chunk_recoder_ctor(void* args, void* res);

/**
 * @brief Construct a new ChunkRecoder object to recode all serialized chunks (new model)
 *
 * @param args {
 *     serializedData *uintptr // pointer to serialized data
 *     time_interval struct { // closed interval [min, max]
 *        min int64
 *        max int64
 *     }
 * }
 * @param res {
 *     chunk_recoder uintptr // pointer to chunk recoder
 * }
 */
void prompp_series_data_serialized_chunk_recoder_ctor(void* args, void* res);

/**
 * @brief Get chunk encoded in prometheus format
 *
 * @param args {
 *     chunk_recoder uintptr // pointer to chunk recoder
 * }
 * @param res {
 *     interval struct {
 *        min int64
 *        max int64
 *     }
 *     series_id     uint32
 *     samples_count uint8
 *     has_more_data bool
 *     data          []byte // SliceView to recoded chunk data
 * }
 */
void prompp_series_data_chunk_recoder_recode_next_chunk(void* args, void* res);

/**
 * @brief Advance ChunkRecoder::ls_id_iterator to next batch
 *
 * @param args {
 *     chunk_recoder uintptr // pointer to chunk recoder
 * }
 *
 * @param res {
 *     hasMoreData bool  // true if chunk recoder has more
 * }
 */
void prompp_series_data_chunk_recoder_next_batch(void* args, void* res);

/**
 * @brief Destruct ChunkRecoder object
 *
 * @param args {
 *     chunk_recoder  uintptr  // pointer to chunk recoder
 * }
 */
void prompp_series_data_chunk_recoder_dtor(void* args);

/**
 * @brief Construct unloader
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 *
 * @param res {
 *     unloader uintptr // pointer to constructed unloader
 * }
 */
void prompp_series_data_data_storage_unloader_ctor(void* args, void* res);

/**
 * @brief Destruct unloader
 *
 * @param args {
 *     unloader uintptr // pointer to constructed unloader
 * }
 *
 */
void prompp_series_data_data_storage_unloader_dtor(void* args);

/**
 * @brief Create data snapshot of unused series
 *
 * @param args {
 *     unloader uintptr // pointer to constructed unloader
 * }
 *
 * @param res {
 *     unloadedData []byte // encoded unload data
 * }
 */
void prompp_series_data_data_storage_unloader_create_snapshot(void* args, void* res);

/**
 * @brief Unload data from DataStorage
 *
 * @param args {
 *     unloader uintptr // pointer to constructed unloader
 * }
 *
 */
void prompp_series_data_data_storage_unloader_unload(void* args);

/**
 * @brief Construct Loader to load previously unqueried series
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 *     labelSetIDs []uint32 // label sets from rejected query
 * }
 *
 *  @param res {
 *     loader uintptr // pointer to loader
 * }
 */
void prompp_series_data_data_storage_loader_ctor(void* args, void* res);

/**
 * @brief Construct RevertableLoader to load previously unqueried series
 *
 * @param args {
 *     lss uintptr            // pointer to constructed label sets
 *     lsIdBatchSize uint32   // size of ls batch for recoding
 *     dataStorage   uintptr  // pointer to constructed data storage
 * }
 *
 *  @param res {
 *     loader uintptr // pointer to loader
 * }
 */
void prompp_series_data_data_storage_revertable_loader_ctor(void* args, void* res);

/**
 * @brief Loads next previously unloaded snapshot of data
 *
 * @param args {
 *     loader uintptr // pointer to loader
 *     buffer []byte // SliceView to unloaded snapshot
 *     is_final bool // flag if this buffer corresponds to the last snapshot
 * }
 */
void prompp_series_data_data_storage_loader_load_next(void* args);

/**
 * @brief Advance RevertableLoader::iterator to next batch
 *
 * @param args {
 *     loader uintptr // pointer to loader
 * }
 *
 * @param res {
 *     hasMoreData bool  // true if chunk recoder has more
 * }
 */
void prompp_series_data_data_storage_revertable_loader_next_batch(void* args, void* res);

/**
 * @brief Destroy Loader object
 *
 * @param args {
 *     loader uintptr // pointer to loader
 * }
 */
void prompp_series_data_data_storage_loader_dtor(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief series data Encoder constructor.
 *
 * @param args {
 *     data_storage uintptr // pointer to constructed data storage
 * }
 *
 * @param res {
 *     encoder uintptr // pointer to constructed encoder
 * }
 */
void prompp_series_data_encoder_ctor(void* args, void* res);

/**
 * @brief adds single series to data storage
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 *     seriesID uint32 // series id
 *     timestamp int64 // timestamp
 *     value float64   // value
 * }
 */
void prompp_series_data_encoder_encode(void* args);

/**
 * @brief adds slice of inner series to data storage
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 *     innerSeriesSlice []*InnerSeries // pointer to inner series slice.
 * }
 */
void prompp_series_data_encoder_encode_inner_series_slice(void* args);

/**
 * @brief merge outdated chunks
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 * }
 */
void prompp_series_data_encoder_merge_out_of_order_chunks(void* args);

/**
 * @brief series data Encoder destructor.
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 * }
 */
void prompp_series_data_encoder_dtor(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Get next series_id in serialized data.
 *
 * @param args {
 *     serializedData uintptr // pointer to serialized data.
 * }
 *
 * @param res {
 *     series_id uint32 // series id (UINT32_MAX if no more series).
 *     chunk_ref uint32 // inner chunk id.
 * }
 */
void prompp_series_data_serialization_serialized_data_next(void* args, void* res);

/**
 * @brief Create a decode iterator for corresponding chunk_ref.
 *
 * @param args {
 *     serializedData uintptr // pointer to serialized data.
 *     chunk_ref uint32 // inner chunk id.
 * }
 *
 */
void prompp_series_data_serialization_serialized_data_iterator_ctor(void* args);

/**
 * @brief Advance decode iterator.
 *
 * @param iterator uintptr // pointer to decode iterator
 *
 */
void prompp_series_data_serialization_serialized_data_iterator_next(void* iterator);

/**
 * @brief Advance decode iterator until referenced sample is gte targetTimestamp.
 *
 * @param args {
 *     iterator uintptr // pointer to decode iterator
 *     targetTimestamp int64 // target timestamp
 * }
 *
 */
void prompp_series_data_serialization_serialized_data_iterator_seek(void* args);

/**
 * @brief Reset a decode iterator for corresponding chunk_ref.
 *
 * @param args {
 *     serializedData uintptr // pointer to serialized data.
 *     iterator uintptr // pointer to decode iterator
 *     chunkRef uint32 // inner chunk id.
 * }
 *
 */
void prompp_series_data_serialization_serialized_data_iterator_reset(void* args);

/**
 * @brief Destroy serialized data object.
 *
 * @param args {
 *     serializedData uintptr // pointer to serialized data.
 * }
 *
 */
void prompp_series_data_serialization_serialized_data_dtor(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Construct a new WAL Decoder
 *
 * @param args {
 *     encoder_version uint8_t // basic encoder version
 * }
 *
 * @param res {
 *     decoder uintptr // pointer to constructed decoder
 * }
 */
void prompp_wal_decoder_ctor(void* args, void* res);

/**
 * @brief Destroy decoder
 *
 * @param args {
 *     decoder uintptr // pointer to constructed decoder
 * }
 */
void prompp_wal_decoder_dtor(void* args);

/**
 * @brief Decode WAL-segment into protobuf message
 *
 * @param args {
 *     decoder uintptr // pointer to constructed decoder
 *     segment []byte  // segment content
 * }
 * @param res {
 *     created_at int64  // timestamp in ns when data was start writed to encoder
 *     encoded_at int64  // timestamp in ns when segment was encoded
 *     samples    uint32 // number of samples in segment
 *     series     uint32 // number of series in segment
 *     segment_id uint32 // processed segment id
 *     earliest_block_sample int64 // min timestamp in block
 *     latest_block_sample inte64 // max timestamp in block
 *     protobuf   []byte // decoded RemoteWrite protobuf content
 *     error      []byte // error string if thrown
 * }
 */
void prompp_wal_decoder_decode(void* args, void* res);

/**
 * @brief Decode WAL-segment into BasicDecoderHashdex
 *
 * @param args {
 *     decoder               uintptr // pointer to constructed decoder
 *     segment               []byte  // segment content
 * }
 * @param res {
 *     created_at            int64   // timestamp in ns when data was start writed to encoder
 *     encoded_at            int64   // timestamp in ns when segment was encoded
 *     samples               uint32  // number of samples in segment
 *     series                uint32  // number of series in segment
 *     segment_id            uint32  // processed segment id
 *     earliest_block_sample int64   // min timestamp in block
 *     latest_block_sample   inte64  // max timestamp in block
 *     hashdex               uintptr // pointer to filled hashdex
 *     cluster               string  // value of label cluster from first sample
 *     replica               string  // value of label __replica__ from first sample
 *     error                 []byte  // error string if thrown
 * }
 */
void prompp_wal_decoder_decode_to_hashdex(void* args, void* res);

/**
 * @brief Decode WAL-segment into BasicDecoderHashdex with metadata for injection metrics.
 *
 * @param args {
 *     decoder               uintptr        // pointer to constructed decoder
 *     meta                  *MetaInjection // pointer to metadata for injection metrics.
 *     segment               []byte         // segment content
 * }
 * @param res {
 *     created_at            int64          // timestamp in ns when data was start writed to encoder
 *     encoded_at            int64          // timestamp in ns when segment was encoded
 *     samples               uint32         // number of samples in segment
 *     series                uint32         // number of series in segment
 *     segment_id            uint32         // processed segment id
 *     earliest_block_sample int64          // min timestamp in block
 *     latest_block_sample   inte64         // max timestamp in block
 *     hashdex               uintptr        // pointer to filled hashdex
 *     cluster               string         // value of label cluster from first sample
 *     replica               string         // value of label __replica__ from first sample
 *     error                 []byte         // error string if thrown
 * }
 */
void prompp_wal_decoder_decode_to_hashdex_with_metric_injection(void* args, void* res);

/**
 * @brief Decode WAL-segment and drop decoded data
 *
 * @param args {
 *     decoder uintptr // pointer to constructed decoder
 *     segment []byte  // segment content
 * }
 * @param res {
 *     segment_id uint32  // last decoded segment id
 *     error   []byte     // error string if thrown
 * }
 */
void prompp_wal_decoder_decode_dry(void* args, void* res);

/**
 * @brief Decode all segments from given stream dump
 *
 * @param args {
 *     decoder    uintptr // pointer to constructed decoder
 *     stream     []byte  // stream dump
 *     segment_id uint32  // id of last segment to decode
 * }
 * @param res {
 *     offset     uint64 // number of read bytes from dump
 *     segment_id uint32 // last decoded segment id
 *     error      []byte // error string if thrown
 * }
 */
void prompp_wal_decoder_restore_from_stream(void* args, void* res);

/**
 * @brief Construct a segment samples storage list
 *
 * @param args {
 *     count  uint64 // storages count
 * }
 *
 * @param res {
 *     storageList []SegmentSamplesStorageList // constructed storage list
 * }
 */
void prompp_wal_segment_samples_storage_list_ctor(void* args, void* res);

/**
 * @brief Add sample to sample storage list
 *
 * @param args {
 *     samplesStorage *SegmentSamplesStorage // pointer to constructed SegmentSamplesStorage
 *     lsId           uint32 // label set id
 *     int64          timestamp // sample timestamp
 *     value          float64   // sample value
 * }
 */
void prompp_wal_segment_samples_storage_add(void* args);

/**
 * @brief Clear sample storage list
 *
 * @param args {
 *     samplesStorage *SegmentSamplesStorage // pointer to constructed SegmentSamplesStorage
 * }
 */
void prompp_wal_segment_samples_storage_clear(void* args);

/**
 * @brief Destroy segment samples storage list
 *
 * @param args {
 *     storageList []SegmentSamplesStorageList
 * }
 */
void prompp_wal_segment_samples_storage_list_dtor(void* args);

//
// OutputDecoder
//

/**
 * @brief Construct a new WAL Output Decoder
 *
 * @param args {
 *     external_labels     []Label // slice with external labels;
 *     stateless_relabeler uintptr // pointer to constructed stateless relabeler;
 *     output_lss          uintptr // pointer to constructed output label sets;
 *     encoder_version     uint8_t // basic encoder version
 * }
 *
 * @param res {
 *     decoder uintptr // pointer to constructed output decoder
 * }
 */
void prompp_wal_output_decoder_ctor(void* args, void* res);

/**
 * @brief Destroy output decoder
 *
 * @param args {
 *     decoder             uintptr // pointer to constructed output decoder
 * }
 */
void prompp_wal_output_decoder_dtor(void* args);

/**
 * @brief Dump output decoder state(output_lss and cache) to slice byte.
 *
 * @param args {
 *     decoder             uintptr // pointer to constructed output decoder
 * }
 *
 * @param res {
 *     dump                []byte  // stream dump
 *     error               []byte  // error string if thrown
 * }
 */
void prompp_wal_output_decoder_dump_to(void* args, void* res);

/**
 * @brief Load from dump(slice byte) output decoder state(output_lss and cache).
 *
 * @param args {
 *     dump                []byte  // stream dump
 *     decoder             uintptr // pointer to constructed output decoder
 * }
 *
 * @param res {
 *     error               []byte  // error string if thrown
 * }
 */
void prompp_wal_output_decoder_load_from(void* args, void* res);

/**
 * @brief decode segment to slice RefSample.
 *
 * @param args {
 *     segment               []byte                 // segment content
 *     decoder               uintptr                // pointer to constructed output decoder
 *     samplesStorage        *SegmentSamplesStorage // pointer to constructed SegmentSamplesStorage
 *     lower_limit_timestamp int64                  // lower limit timestamp
 * }
 *
 * @param res {
 *     max_timestamp         int64       // max timestamp in slice RefSample
 *     outdated_sample_count uint32      // count of dropped samples on outdated
 *     dropped_sample_count  uint32      // count of dropped samples on relabeling rules
 *     add_series_count      uint32      // count of add series on relabeling rules
 *     dropped_series_count  uint32      // count of dropped series on relabeling rules
 *     sample_count         uint32       // count of samples added to samplesStorage
 *     error                 []byte      // error string if thrown
 * }
 */
void prompp_wal_output_decoder_decode(void* args, void* res);

#ifdef __cplusplus
}  // extern "C"
#endif
#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Basic encoder version
 *
 * @param res {
 *     encoders_version uint8_t // basic encoders version
 * }
 */
void prompp_wal_encoders_version(void* res);

/**
 * @brief Construct a new WAL Encoder
 *
 * @param args {
 *     shard_id   uint16 // shard number
 *     log_shards uint8  // logarithm to the base 2 of total shards count
 * }
 * @param res {
 *     encoder uintptr // pointer to constructed encoder
 * }
 */
void prompp_wal_encoder_ctor(void* args, void* res);

/**
 * @brief Destroy encoder
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 * }
 */
void prompp_wal_encoder_dtor(void* args);

/**
 * @brief Add data to current segment
 *
 * @param args {
 *     encoder uintptr      // pointer to constructed encoder
 *     hashdex uintptr      // pointer to filled hashdex
 * }
 * @param res {
 *     samples            uint32  // number of samples in segment
 *     series             uint32  // number of series in segment
 *     earliest_timestamp int64   // minimal sample timestamp in segment
 *     latest_timestamp   int64   // maximal sample timestamp in segment
 *     remainder_size     uint32  // rest of internal buffers capacity
 *     error              []byte  // error string if thrown
 * }
 */
void prompp_wal_encoder_add(void* args, void* res);

/**
 * @brief Add inner series to current segment
 *
 * @param args {
 *     incoming_inner_series []InnerSeries // go slice with incoming InnerSeries;
 *     encoder               uintptr        // pointer to constructed encoder;
 * }
 * @param res {
 *     samples            uint32  // number of samples in segment
 *     series             uint32  // number of series in segment
 *     earliest_timestamp int64   // minimal sample timestamp in segment
 *     latest_timestamp   int64   // maximal sample timestamp in segment
 *     remainder_size     uint32  // rest of internal buffers capacity
 *     error              []byte  // error string if thrown
 * }
 */
void prompp_wal_encoder_add_inner_series(void* args, void* res);

/**
 * @brief Add relabeled series to current segment
 *
 * @param args {
 *     incoming_relabeled_series []*RelabeledSeries // go slice with incoming RelabeledSeries;
 *     encoder                   uintptr            // pointer to constructed encoder
 *     relabeler_state_update    uintptr            // pointer to constructed RelabelerStateUpdate;
 * }
 * @param res {
 *     earliest_timestamp int64   // minimal sample timestamp in segment
 *     latest_timestamp   int64   // maximal sample timestamp in segment
 *     allocated_memory   uint64  // size of allocated memory for label sets;
 *     samples            uint32  // number of samples in segment
 *     series             uint32  // number of series in segment
 *     remainder_size     uint32  // rest of internal buffers capacity
 *     error              []byte  // error string if thrown
 * }
 */
void prompp_wal_encoder_add_relabeled_series(void* args, void* res);

/**
 * @brief Add data to current segment and mark as stale obsolete series
 *
 * @param args {
 *     encoder      uintptr // pointer to constructed encoder
 *     hashdex      uintptr // pointer to filled hashdex
 *     hashdex_type uint8   // type of hashdex
 *     stale_ts     int64   // timestamp for StaleNaNs
 *     source_state uintptr // pointer to source state (null on first call)
 * }
 * @param res {
 *     samples            uint32  // number of samples in segment
 *     series             uint32  // number of series in segment
 *     earliest_timestamp int64   // minimal sample timestamp in segment
 *     latest_timestamp   int64   // maximal sample timestamp in segment
 *     remainder_size     uint32  // rest of internal buffers capacity
 *     source_state       uintptr // pointer to internal source state
 *     error              []byte  // error string if thrown
 * }
 */
void prompp_wal_encoder_add_with_stale_nans(void* args, void* res);

/**
 * @brief Destroy source state and mark all series as stale
 *
 * @param args {
 *     encoder      uintptr // pointer to constructed encoder
 *     stale_ts     int64   // timestamp for StaleNaNs
 *     source_state uintptr // pointer to source state (null on first call)
 * }
 * @param res {
 *     samples            uint32  // number of samples in segment
 *     series             uint32  // number of series in segment
 *     earliest_timestamp int64   // minimal sample timestamp in segment
 *     latest_timestamp   int64   // maximal sample timestamp in segment
 *     remainder_size     uint32  // rest of internal buffers capacity
 *     error              []byte  // error string if thrown
 * }
 */
void prompp_wal_encoder_collect_source(void* args, void* res);

/**
 * @brief Flush segment
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 * }
 * @param res {
 *     samples            uint32  // number of samples in segment
 *     series             uint32  // number of series in segment
 *     earliest_timestamp int64   // minimal sample timestamp in segment
 *     latest_timestamp   int64   // maximal sample timestamp in segment
 *     remainder_size     uint32  // rest of internal buffers capacity
 *     segment            []byte  // segment content
 *     error              []byte  // error string if thrown
 * }
 */
void prompp_wal_encoder_finalize(void* args, void* res);

//
// EncoderLightweight
//

/**
 * @brief Construct a new WAL EncoderLightweight
 *
 * @param args {
 *     shardID            uint16  // shard number
 *     logShards          uint8   // logarithm to the base 2 of total shards count
 * }
 * @param res {
 *     encoderLightweight uintptr // pointer to constructed encoder
 * }
 */
void prompp_wal_encoder_lightweight_ctor(void* args, void* res);

/**
 * @brief Destroy EncoderLightweight
 *
 * @param args {
 *     encoderLightweight uintptr // pointer to constructed encoder
 * }
 */
void prompp_wal_encoder_lightweight_dtor(void* args);

/**
 * @brief Add data to current segment
 *
 * @param args {
 *     encoderLightweight uintptr      // pointer to constructed encoder
 *     hashdex            uintptr      // pointer to filled hashdex
 * }
 * @param res {
 *     earliestTimestamp  int64        // minimal sample timestamp in segment
 *     latestTimestamp    int64        // maximal sample timestamp in segment
 *     allocatedMemory    uint64       // size of allocated memory for label sets;
 *     samples            uint32       // number of samples in segment
 *     series             uint32       // number of series in segment
 *     remainderSize      uint32       // rest of internal buffers capacity
 *     error              []byte       // error string if thrown
 * }
 */
void prompp_wal_encoder_lightweight_add(void* args, void* res);

/**
 * @brief Add inner series to current segment
 *
 * @param args {
 *     incomingInnerSeries []InnerSeries // go slice with incoming InnerSeries;
 *     encoderLightweight  uintptr        // pointer to constructed encoder;
 * }
 * @param res {
 *     earliestTimestamp   int64          // minimal sample timestamp in segment
 *     latestTimestamp     int64          // maximal sample timestamp in segment
 *     allocatedMemory     uint64         // size of allocated memory for label sets;
 *     samples             uint32         // number of samples in segment
 *     series              uint32         // number of series in segment
 *     remainderSize       uint32         // rest of internal buffers capacity
 *     error               []byte         // error string if thrown
 * }
 */
void prompp_wal_encoder_lightweight_add_inner_series(void* args, void* res);

/**
 * @brief Add relabeled series to current segment
 *
 * @param args {
 *     incomingRelabeledSeries []*RelabeledSeries // go slice with incoming RelabeledSeries;
 *     encoderLightweight      uintptr            // pointer to constructed encoder
 *     relabelerStateUpdate    uintptr            // pointer to constructed RelabelerStateUpdate;
 * }
 * @param res {
 *     earliestTimestamp       int64              // minimal sample timestamp in segment
 *     latestTimestamp         int64              // maximal sample timestamp in segment
 *     allocatedMemory         uint64             // size of allocated memory for label sets;
 *     samples                 uint32             // number of samples in segment
 *     series                  uint32             // number of series in segment
 *     remainderSize           uint32             // rest of internal buffers capacity
 *     error                   []byte             // error string if thrown
 * }
 */
void prompp_wal_encoder_lightweight_add_relabeled_series(void* args, void* res);

/**
 * @brief Flush segment
 *
 * @param args {
 *     encoderLightweight uintptr // pointer to constructed encoder
 * }
 * @param res {
 *     earliestTimestamp  int64   // minimal sample timestamp in segment
 *     latestTimestamp    int64   // maximal sample timestamp in segment
 *     allocatedMemory    uint64  // size of allocated memory for label sets;
 *     samples            uint32  // number of samples in segment
 *     series             uint32  // number of series in segment
 *     remainderSize      uint32  // rest of internal buffers capacity
 *     error              []byte  // error string if thrown
 * }
 */
void prompp_wal_encoder_lightweight_finalize(void* args, void* res);

#ifdef __cplusplus
}  // extern "C"
#endif
#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Destroy hashdex
 *
 * @param args {
 *     hashdex uintptr // pointer to constructed hashdex
 * }
 */
void prompp_wal_hashdex_dtor(void* args);

/**
 * @brief Construct a new WAL Hashdex
 *
 * @param args { // limits for incoming data
 *     max_label_name_length          uint32
 *     max_label_value_length         uint32
 *     max_label_names_per_timeseries uint32
 *     max_timeseries_count           uint64
 *     max_pb_size_in_bytes           uint64
 * }
 * @param res {
 *     hashdex uintptr // pointer to constructed hashdex
 * }
 */
void prompp_wal_protobuf_hashdex_ctor(void* args, void* res);

/**
 * @brief Fill hashdex from compressed via snappy protobuf
 *
 * Hashdex only indexing protobuf and doesn't copy all data.
 * Caller should preserve original protobuf content at the same
 * memory address to use hashdex in next call.
 *
 * @param args {
 *     hashdex             uintptr // pointer to constructed hashdex
 *     compressed_protobuf []byte  // compressed via snappy RemoteWrite protobuf content
 * }
 * @param res {
 *     // this data is a view over protobuf memory and shouldn't be destroyed explicitely
 *     cluster string // value of label cluster from first sample
 *     replica string // value of label __replica__ from first sample
 *     error   []byte // error string if thrown
 * }
 */
void prompp_wal_protobuf_hashdex_snappy_presharding(void* args, void* res);

/**
 * @brief Get parsed metadata
 *
 * @param args {
 *     hashdex uintptr
 * }
 * @param res {
 *     metadata []struct {
 *        metric_name string
 *        text string
 *        type uint32
 *     }
 * }
 */
void prompp_wal_protobuf_hashdex_get_metadata(void* args, void* res);

/**
 * @brief Construct a new WAL GoModelHashdex
 *
 * @param args { // limits for incoming data
 *     max_label_name_length          uint32
 *     max_label_value_length         uint32
 *     max_label_names_per_timeseries uint32
 *     max_timeseries_count           uint64
 * }
 * @param res {
 *     hashdex uintptr // pointer to constructed hashdex
 * }
 */
void prompp_wal_go_model_hashdex_ctor(void* args, void* res);

/**
 * @brief Fill hashdex from Go memory
 *
 * Hashdex only indexing go memory (model.TimeSeries) and doesn't copy all data.
 * Caller should preserve original protobuf content at the same
 * memory address to use hashdex in next call.
 *
 * @param args {
 *     hashdex  uintptr // pointer to constructed hashdex
 *     data     []model.TimeSeries  // Go content
 * }
 * @param res {
 *     // this data is a view over go memory and shouldn't be destroyed explicitely
 *     cluster string // value of label cluster from first sample
 *     replica string // value of label __replica__ from first sample
 *     error   []byte // error string if thrown
 * }
 */
void prompp_wal_go_model_hashdex_presharding(void* args, void* res);

/**
 * @brief Construct a new PromPP::WAL::hashdex::Scraper based on Prometheus parser
 *
 * @param res {
 *     hashdex uintptr // pointer to constructed hashdex
 * }
 */
void prompp_wal_prometheus_scraper_hashdex_ctor(void* res);

/**
 * @brief Parse scraped buffer
 *
 * @param args {
 *     hashdex           uintptr
 *     buffer            string // buffer will be modified by parser
 *     default_timestamp int64
 * }
 * @param res {
 *     error uint32 // value of PromPP::WAL::hashdex::Scraper::Error
 * }
 */
void prompp_wal_prometheus_scraper_hashdex_parse(void* args, void* res);

/**
 * @brief Get scraped metadata
 *
 * @param args {
 *     hashdex uintptr
 * }
 * @param res {
 *     metadata []struct {
 *        metric_name string
 *        text string
 *        type uint32
 *     }
 * }
 */
void prompp_wal_prometheus_scraper_hashdex_get_metadata(void* args, void* res);

/**
 * @brief Construct a new PromPP::WAL::hashdex::Scraper based on OpenMetrics parser
 *
 * @param res {
 *     hashdex uintptr // pointer to constructed hashdex
 * }
 */
void prompp_wal_open_metrics_scraper_hashdex_ctor(void* res);

/**
 * @brief Parse scraped buffer
 *
 * @param args {
 *     hashdex           uintptr
 *     buffer            string // buffer will be modified by parser
 *     default_timestamp int64
 * }
 * @param res {
 *     error uint32 // value of PromPP::WAL::hashdex::Scraper::Error
 * }
 */
void prompp_wal_open_metrics_scraper_hashdex_parse(void* args, void* res);

/**
 * @brief Get scraped metadata
 *
 * @param args {
 *     hashdex uintptr
 * }
 * @param res {
 *     metadata []struct {
 *        metric_name string
 *        text string
 *        type uint32
 *     }
 * }
 */
void prompp_wal_open_metrics_scraper_hashdex_get_metadata(void* args, void* res);

/**
 * @brief Construct a new PromPP::WAL::hashdex::GoHead hashdex
 *
 * @param res {
 *     hashdex uintptr // pointer to constructed hashdex
 * }
 */
void prompp_wal_go_head_hashdex_ctor(void* res);

/**
 * @brief Fill hashdex from Go Head
 *
 * @param args {
 *     hashdex  uintptr // pointer to constructed hashdex
 *     lss uintptr      // pointer to constructed lss
 *     dataStorage uintptr // pointer to constructed DataStorage
 * }
 */
void prompp_wal_go_head_hashdex_presharding(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief return determined flavor
 *
 * @param res {
 *   flavor string
 * }
 */
void prompp_get_flavor(void* res);

#ifdef __cplusplus
}
#endif
