#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Construct a multi-series decode iterator over the given series ids.
 *
 * @param args {
 *     iterator uintptr // pointer to storage of size Sizeof_MultiSeriesDecodeIterator (placement new).
 *     serializedData uintptr // pointer to serialized data.
 *     seriesIDs []uint32 // slice view of series ids to use in iterator.
 * }
 */
void prompp_series_data_serialization_serialized_data_multi_series_iterator_ctor(void* args);

/**
 * @brief Reset a multi-series decode iterator into the given series ids.
 *
 * @param args {
 *     iterator uintptr // pointer to a constructed MultiSeriesDecodeIterator.
 *     serializedData uintptr // pointer to serialized data.
 *     seriesIDs []uint32 // slice view of series ids to use in iterator.
 * }
 */
void prompp_series_data_serialization_serialized_data_multi_series_iterator_reset(void* args);

/**
 * @brief Advance multi-series decode iterator.
 *
 * @param iterator uintptr // pointer to multi-series decode iterator
 */
void prompp_series_data_serialization_serialized_data_multi_series_iterator_next(void* iterator);

/**
 * @brief Destroy multi-series decode iterator (call before reusing).
 *
 * @param iterator uintptr // pointer to multi-series decode iterator
 */
void prompp_series_data_serialization_serialized_data_multi_series_iterator_dtor(void* iterator);

#ifdef __cplusplus
}  // extern "C"
#endif
