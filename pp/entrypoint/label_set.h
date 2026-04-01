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
 *     snapshot  uintptr                      // pointer to constructed snapshot;
 *     ls_id     uint32                       // series id
 * }
 *
 * @param res {
 *     label_set []struct{key, value String}  // label sets
 * }
 */
void prompp_label_set_serialize_from_snapshot(void* args, void* res);

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
