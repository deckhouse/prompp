#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Construct a new Primitives label sets.
 *
 * @param args {
 *     lss_type uint32 // type of lss;
 * }
 *
 * @param res {
 *     lss uintptr     // pointer to constructed label sets;
 * }
 */
void prompp_primitives_lss_ctor(void* args, void* res);

/**
 * @brief Construct a new Primitives label sets.
 *
 * @param args {
 *     source      uintptr // pointer to source label sets
 *     destination uintptr // pointer to destination label sets
 * }
 *
 */
void prompp_primitives_lss_copy_added_series(void* args);

/**
 * @brief Destroy Primitives label sets.
 *
 * @param args {
 *     lss uintptr // pointer of label sets;
 * }
 */
void prompp_primitives_lss_dtor(void* args);

/**
 * @brief return size of allocated memory for label sets.
 *
 * @param args {
 *     lss uintptr             // pointer to constructed label sets;
 * }
 *
 * @param res {
 *     allocated_memory uint64 // size of allocated memory for label sets;
 * }
 */
void prompp_primitives_lss_allocated_memory(void* args, void* res);

/**
 * @brief insert label set into lss
 *
 * @param args {
 *     lss uintptr              // pointer to constructed lss;
 *     label_set model.LabelSet // label set
 * }
 *
 * @param res {
 *     ls_id uint32 // inserted (or found) label set id
 * }
 */
void prompp_primitives_lss_find_or_emplace(void* args, void* res);

/**
 * @brief insert label set into lss
 *
 * @param args {
 *     lss        uintptr        // pointer to constructed lss;
 *     label_set  model.LabelSet // label set
 * }
 *
 * @param res {
 *     lss_ro_ptr uintptr        // readonly copy of lss
 *     ls_id      uint32         // inserted (or found) label set id
 * }
 */
void prompp_primitives_lss_find_or_emplace_label_set(void* args, void* res);

/**
 * @brief insert label set into lss
 *
 * @param args {
 *     lss        uintptr        // pointer to constructed lss;
 *     label_set  model.LabelSet // label set
 * }
 *
 * @param res {
 *     lss_ro_ptr uintptr        // readonly copy of lss
 *     ls_id      uint32         // inserted (or found) label set id
 *     has        bool           // is the label set found
 * }
 */
void prompp_primitives_lss_find(void* args, void* res);

/**
 * @brief query series from lss
 *
 * @param args {
 *     lss uintptr                         // pointer to constructed queryable lss;
 *     label_matchers []model.LabelMatcher // label matchers
 *     query_source uint32                 // query source (rule, federate, other)
 * }
 *
 * @param res {
 *     matches           []uint32 // matched series ids
 *     label_set_lengths []uint16 // slice of series label set length
 *     lss_copy          uintptr  // readonly copy of lss
 *     status            uint32   // query status
 * }
 */
void prompp_primitives_lss_query(void* args, void* res);

/**
 * @brief free label set matches returned by prompp_primitives_lss_query
 *
 * @param args {
 *     matches           []uint32 // matched series ids
 *     label_set_lengths []uint16 // slice of series label set length
 *     status            uint32   // query status
 * }
 */
void prompp_primitives_lss_query_result_free(void* args);

/**
 * @brief get label sets by series id
 *
 * @param args {
 *     lss uintptr    // pointer to constructed lss;
 *     ls_id []uint32 // series ids
 * }
 *
 * @param res {
 *     label_sets [][]struct {key, value String} // label sets
 * }
 */
void prompp_primitives_lss_get_label_sets(void* args, void* res);

/**
 * @brief free label sets returned by prompp_primitives_lss_get_label_sets
 *
 * @param args {
 *     label_sets [][]struct {key, value String} // label set
 * }
 */
void prompp_primitives_lss_free_label_sets(void* args);

/**
 * @brief return size of allocated memory for label sets.
 *
 * @param args {
 *     lss uintptr                         // pointer to constructed queryable lss;
 *     label_matchers []model.LabelMatcher // label matchers
 * }
 *
 * @param res {
 *     status uint32   // query status
 *     names  []string // Slice of string freed by freeBytes in GO pointed to lss memory, so it may be invalid after mutating lss state
 * }
 */
void prompp_primitives_lss_query_label_names(void* args, void* res);

/**
 * @brief return size of allocated memory for label sets.
 *
 * @param args {
 *     lss uintptr                         // pointer to constructed queryable lss;
 *     label_name string                   // label name
 *     label_matchers []model.LabelMatcher // label matchers
 * }
 *
 * @param res {
 *     status uint32   // query status
 *     values []string // Slice of string freed by freeBytes in GO pointed to lss memory, so it may be invalid after mutating lss state
 * }
 */
void prompp_primitives_lss_query_label_values(void* args, void* res);

//
// label_sets
//

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
void prompp_primitives_label_set_length(void* args, void* res);

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
void prompp_primitives_label_set_serialize(void* args, void* res);

/**
 * @brief free label set returned by prompp_primitives_label_set_serialize
 *
 * @param args {
 *     label_set []struct{key, value String} // label set
 * }
 */
void prompp_primitives_label_set_free(void* args);

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
void prompp_primitives_label_set_get_value(void* args, void* res);

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
void prompp_primitives_label_set_has_label_name(void* args, void* res);

/**
 * @brief returns a hash value for the label set from lss by series id
 *
 * @param args {
 *     lss         uintptr                      // pointer to constructed lss;
 *     ls_id       uint32                       // series id
 * }
 *
 * @param res {
 *     hash        uint64                       // hash sum
 * }
 */
void prompp_primitives_label_set_hash(void* args, void* res);

/**
 * @brief returns a hash value for the labels matching the provided names for label set from lss by series id
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
void prompp_primitives_label_set_hash_for_labels(void* args, void* res);

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
void prompp_primitives_label_set_hash_without_labels(void* args, void* res);

/**
 * @brief returns whether the two label sets are equal.
 *
 * @param args {
 *     lss_a         uintptr                      // pointer to constructed lss a;
 *     lss_b         uintptr                      // pointer to constructed lss b;
 *     ls_id_a       uint32                       // series id a
 *     ls_id_b       uint32                       // series id b
 * }
 *
 * @param res {
 *     is_equal      bool                         // is equal?
 * }
 */
void prompp_primitives_label_set_equal(void* args, void* res);

/**
 * @brief Compare compares the two label sets.
 *
 * @param args {
 *     lss_a         uintptr                      // pointer to constructed lss a;
 *     lss_b         uintptr                      // pointer to constructed lss b;
 *     ls_id_a       uint32                       // series id a
 *     ls_id_b       uint32                       // series id b
 * }
 *
 * @param res {
 *     result        int64                         // compare result
 * }
 */
void prompp_primitives_label_set_compare(void* args, void* res);

#ifdef __cplusplus
}  // extern "C"
#endif
