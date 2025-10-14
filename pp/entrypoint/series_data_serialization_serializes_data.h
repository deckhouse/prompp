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
 * }
 */
void prompp_series_data_serialization_serialized_data_next(void* args, void* res);

/**
 * @brief Create a decode iterator for current series_id (returned by the last call of _next())
 *
 * @param args {
 *     serializedData uintptr // pointer to serialized data.
 * }
 *
 * @param res {
 *     iterator uintptr // pointer to constructed decode iterator.
 * }
 */
void prompp_series_data_serialization_serialized_data_iterator(void* args, void* res);

/**
 * @brief Advance decode iterator.
 *
 * @param args {
 *     iterator uintptr // pointer to decode iterator
 * }
 *
 * @param res {
 *     has_data bool    // is iterator has more data to decode.
 *      timestamp int64 // sample timestamp
 *      value float64   // sample value
 * }
 */
void prompp_series_data_serialization_serialized_data_iterator_next(void* args, void* res);

/**
 * @brief Destroy decode iterator.
 *
 * @param args {
 *     iterator uintptr // pointer to decode iterator
 * }
 *
 */
void prompp_series_data_serialization_serialized_data_iterator_dtor(void* args);

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
