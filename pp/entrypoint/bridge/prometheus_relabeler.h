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
