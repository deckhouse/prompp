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
 * @brief Queries data storage and serializes result.
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 *     query DataStorageQuery // query
 * }
 *
 * @param res {
 *     serializedData []byte // serialized data
 * }
 */
void prompp_series_data_data_storage_query(void* args, void* res);

void prompp_series_data_data_storage_query_final(void* args);

/**
 * @brief return samples at given timestamp for label sets.
 *
 * @param args {
 *        dataStorage uintptr    // pointer to constructed data storage
 *        labelSetIDs []uint32   // series ids
 *        timestamp   int64      // timestamp
 *        samples     []struct { // pre-allocated samples slice
 *                timestamp int64
 *                value     float64
 *        }
 * }
 */
void prompp_series_data_data_storage_instant_query(void* args);

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
 *     dataStorage   uintptr  // pointer to constructed data storage
 *     time_interval struct { closed interval [min, max]
 *        min int64
 *        max int64
 *     }
 * }
 * @param res {
 *     chunk_recoder uintptr // pointer to chunk recoder
 * }
 */
void prompp_series_data_chunk_recoder_ctor(void* args, void* res);

/**
 * @brief Construct a new ChunkRecoder object for recode all serialized chunks
 *
 * @param args {
 *     buffer []byte // SliceView to serialized chunks buffer
 *     time_interval struct { closed interval [min, max]
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
 * @brief Destruct ChunkRecoder object
 *
 * @param args {
 *     chunk_recoder  uintptr  // pointer to chunk recoder
 * }
 */
void prompp_series_data_chunk_recoder_dtor(void* args);

/**
 * @brief Unloads data of unused series
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 *
 * @param res {
 *     unloadedData []byte // encoded unload data
 * }
 */
void prompp_series_data_data_storage_unload(void* args, void* res);

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
 * @brief Loads next previously unloaded snapshot of data
 *
 * @param args {
 *     loader uintptr // pointer to loader
 *     buffer []byte // SliceView to unloaded snapshot
 * }
 */
void prompp_series_data_data_storage_loader_load_next(void* args);

/**
 * @brief Finalize a loading process after no snapshots left
 *
 * @param args {
 *     loader uintptr // pointer to loader
 * }
 */
void prompp_series_data_data_storage_loader_load_finalize(void* args);

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
