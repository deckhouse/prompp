#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief adds single series sample to data storage
 *
 * @param args {
 *     data_storage uintptr // pointer to constructed data storage
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
 *     data_storage uintptr // pointer to constructed data storage
 *     innerSeriesSlice []*InnerSeries // pointer to inner series slice.
 * }
 */
void prompp_series_data_encoder_encode_inner_series_slice(void* args);

/**
 * @brief merge outdated chunks
 *
 * @param args {
 *     data_storage uintptr // pointer to constructed data storage
 * }
 */
void prompp_series_data_encoder_merge_out_of_order_chunks(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
