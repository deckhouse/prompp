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
 * @param args {
 *     iterator uintptr // pointer to decode iterator
 * }
 *
 */
void prompp_series_data_serialization_serialized_data_iterator_next(void* args);

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
