#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief get length label set by series id
 *
 * @param args {
 *     lss              uintptr // pointer to constructed lss;
 *     ls_id            uint32  // series id
 *     drop_metric_name bool    // flag drop metric_name;
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
 *     lss              uintptr // pointer to constructed lss;
 *     ls_id            uint32  // series id;
 *     drop_metric_name bool    // flag drop metric_name;
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

/**
 * @brief get size in bytes needed for Bytes method
 *
 * @param args {
 *     lss              uintptr   // pointer to constructed lss;
 *     ls_id            uint32    // series id
 *     drop_metric_name bool      // flag drop metric_name;
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
 *     lss              uintptr   // pointer to constructed lss;
 *     ls_id            uint32    // series id
 *     drop_metric_name bool      // flag drop metric_name;
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
 *     lss              uintptr   // pointer to constructed lss;
 *     names            []string  // names slice
 *     ls_id            uint32    // series id
 *     drop_metric_name bool      // flag drop metric_name;
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
 *     lss              uintptr   // pointer to constructed lss;
 *     names            []string  // names slice
 *     ls_id            uint32    // series id
 *     drop_metric_name bool      // flag drop metric_name;
 * }
 *
 * @param res {
 *     bytes []byte
 * }
 */
void prompp_label_set_bytes_without_labels(void* args, void* res);

/**
 * @brief get label value by label name from lss by series id
 *
 * @param args {
 *     lss         uintptr                      // pointer to constructed lss;
 *     label_name  string                       // label name
 *     ls_id       uint32                       // series id
 * }
 *
 * @param res {
 *     label_value string                       // label value
 * }
 */
void prompp_label_set_get_value(void* args, void* res);

/**
 * @brief has label name in label set from lss by series id
 *
 * @param args {
 *     lss         uintptr                      // pointer to constructed lss;
 *     label_name  string                       // label name
 *     ls_id       uint32                       // series id
 * }
 *
 * @param res {
 *     is_has      bool                         // has?
 * }
 */
void prompp_label_set_has_label_name(void* args, void* res);

/**
 * @brief returns whether ls has duplicate label names in label set from lss by series id
 *
 * @param args {
 *     lss              uintptr // pointer to constructed lss;
 *     ls_id            uint32  // series id
 *     drop_metric_name bool    // flag drop metric_name;
 * }
 *
 * @param res {
 *     label_name       string  // label name
 *     is_has           bool    // has?
 * }
 */
void prompp_label_set_has_duplicate_label_names(void* args, void* res);

/**
 * @brief returns a hash value for the label set from lss by series id
 *
 * @param args {
 *     lss              uintptr // pointer to constructed lss;
 *     ls_id            uint32  // series id
 *     drop_metric_name bool    // flag drop metric_name;
 * }
 *
 * @param res {
 *     hash             uint64  // hash sum
 * }
 */
void prompp_label_set_hash(void* args, void* res);

/**
 * @brief returns a hash value for the labels matching the provided names for label set from lss by series id
 *
 * @param args {
 *     lss              uintptr  // pointer to constructed lss;
 *     label_names      []string // label names for filter;
 *     ls_id            uint32   // series id;
 *     drop_metric_name bool     // flag drop metric_name;
 * }
 *
 * @param res {
 *     hash             uint64   // hash sum
 * }
 */
void prompp_label_set_hash_for_labels(void* args, void* res);

/**
 * @brief returns a hash value for all labels except those matching the provided names for label set from lss by series id
 *
 * @param args {
 *     lss         uintptr                      // pointer to constructed lss;
 *     label_names []string                     // label names for filter
 *     ls_id       uint32                       // series id
 * }
 *
 * @param res {
 *     hash        uint64                       // hash sum
 * }
 */
void prompp_label_set_hash_without_labels(void* args, void* res);

/**
 * @brief returns whether the two label sets are equal.
 *
 * @param args {
 *     lss_a              uintptr                 // pointer to constructed lss a;
 *     lss_b              uintptr                 // pointer to constructed lss b;
 *     ls_id_a            uint32                  // series id a;
 *     ls_id_b            uint32                  // series id b;
 *     drop_metric_name_a bool                    // drop metric_name a;
 *     drop_metric_name_b bool                    // drop metric_name b;
 *     is_equal           bool                    // is equal?
 * }
 */
void prompp_label_set_equal(void* args);

/**
 * @brief Compare compares the two label sets.
 *
 * @param args {
 *     lss_a              uintptr                 // pointer to constructed lss a;
 *     lss_b              uintptr                 // pointer to constructed lss b;
 *     ls_id_a            uint32                  // series id a
 *     ls_id_b            uint32                  // series id b
 *     drop_metric_name_a bool                    // drop metric_name a;
 *     drop_metric_name_b bool                    // drop metric_name b;
 * }
 *
 * @param res {
 *     result             int64                   // compare result
 * }
 */
void prompp_label_set_compare(void* args, void* res);

/**
 * @brief returns a hash value for the label set from builder
 *
 * @param args {
 *     readonly_lss uintptr  // pointer to constructed lss;
 *     sorted_add   []Label  // slice of sorted by name labels
 *     sorted_del   []string // slice of sorted label names
 *     ls_id        uint32   // series id
 * }
 *
 * @param res {
 *     hash         uint64   // hash sum
 *     empty        bool     // empty labelset in builder
 * }
 */
void prompp_label_set_from_builder_hash(void* args, void* res);

/**
 * @brief returns whether the label set and the label set from builder are equal.
 *
 * @param args {
 *     snapshot           uintptr  // pointer to constructed snapshot;
 *     builder_snapshot   uintptr  // pointer to constructed snapshot from builder;
 *     builder_sorted_add []Label  // slice of sorted by name labels
 *     builder_sorted_del []string // slice of sorted label names
 *     builder_ls_id      uint32   // series id from builder;
 *     ls_id              uint32   // series id;
 * }
 *
 * @param res {
 *     eq                 bool     // equal;
 * }
 */
void prompp_label_set_equal_with_builder(void* args, void* res);

#ifdef __cplusplus
}  // extern "C"
#endif
