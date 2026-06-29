#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Create a samples iterator for corresponding chunk_ref.
 *
 * @param args {
 *     serializedData uintptr // pointer to serialized data.
 *     chunk_ref uint32 // inner chunk id.
 * }
 *
 */
void prompp_series_data_serialization_serialized_data_samples_iterator_ctor(void* args);

/**
 * @brief Advance samples iterator.
 *
 * @param iterator uintptr // pointer to samples iterator
 *
 */
void prompp_series_data_serialization_serialized_data_samples_iterator_next(void* iterator);

/**
 * @brief Advance samples iterator until referenced sample is gte targetTimestamp.
 *
 * @param args {
 *     iterator uintptr // pointer to samples iterator
 *     targetTimestamp int64 // target timestamp
 * }
 *
 */
void prompp_series_data_serialization_serialized_data_samples_iterator_seek(void* args);

/**
 * @brief Reset a samples iterator for corresponding chunk_ref.
 *
 * @param args {
 *     serializedData uintptr // pointer to serialized data.
 *     iterator uintptr // pointer to samples iterator
 *     chunkRef uint32 // inner chunk id.
 * }
 *
 */
void prompp_series_data_serialization_serialized_data_samples_iterator_reset(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
