#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Create a aggregation iterator for corresponding chunk_ref.
 *
 * @param args {
 *     serializedData uintptr // pointer to serialized data.
 *     chunk_ref uint32 // inner chunk id.
 * }
 *
 */
void prompp_series_data_serialization_serialized_data_aggregation_iterator_ctor(void* args);

/**
 * @brief Advance aggregation iterator.
 *
 * @param iterator uintptr // pointer to aggregation iterator
 *
 */
void prompp_series_data_serialization_serialized_data_aggregation_iterator_next(void* iterator);

/**
 * @brief Reset a aggregation iterator for corresponding chunk_ref.
 *
 * @param args {
 *     serializedData uintptr // pointer to serialized data.
 *     iterator uintptr // pointer to aggregation iterator
 *     chunkRef uint32 // inner chunk id.
 * }
 *
 */
void prompp_series_data_serialization_serialized_data_aggregation_iterator_reset(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
