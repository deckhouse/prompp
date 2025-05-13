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

#ifdef __cplusplus
}  // extern "C"
#endif
