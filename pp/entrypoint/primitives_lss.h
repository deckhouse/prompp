#ifdef __cplusplus
extern "C" {
#endif

#include <stdint.h>

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
 *     ls_id uint32                  // inserted (or found) label set id
 *     bool  lss_has_reallocations   // true if lss has reallocations
 * }
 */
void prompp_primitives_lss_find_or_emplace(void* args, void* res);

/**
 * @brief insert label set builder into lss
 *
 * @param args {
 *     lss uintptr                    // pointer to constructed lss;
 *     builder struct {
 *        snapshot     uintptr        // pointer to constructed snapshot lss;
 *        ls_id        uint32         // series id
 *        sorted_add   []model.Label  // slice of sorted by name labels
 *        sorted_del   []string       // slice of sorted label names
 *     }
 * }
 *
 * @param res {
 *     ls_id uint32                   // inserted (or found) label set id
 *     bool  lss_has_reallocations    // true if lss has reallocations
 * }
 */
void prompp_primitives_lss_find_or_emplace_builder(void* args, void* res);

/**
 * @brief query selector from lss for label matchers
 *
 * @param args {
 *     lss uintptr                         // pointer to constructed queryable lss;
 *     label_matchers []model.LabelMatcher // label matchers
 * }
 *
 * @param res {
 *     selector uintptr // constructed selector
 *     status   uint32  // query status
 * }
 */
void prompp_primitives_lss_query_selector(void* args, void* res);

/**
 * @brief query selector from lss for label matchers
 *
 * @param args {
 *     snapshot uintptr // pointer to snapshot
 *     selector uintptr // pointer to constructed selector
 * }
 *
 * @param res {
 *     matches           []uint32 // matched series ids
 *     label_set_lengths []uint16 // slice of series label set length
 *     status            uint32   // query status
 * }
 */
void prompp_primitives_snapshot_query(void* args, void* res);

/**
 * @brief free label set matches returned by prompp_primitives_snapshot_query
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

/**
 * @brief Resolve label name strings to ids for a queryable LSS.
 *
 * @param args {
 *     lss    uintptr    // pointer to constructed queryable lss
 *     names  []string   // label names
 * }
 *
 * @param res {
 *     ids []uint32  // snapshot of lss
 * }
 */
void prompp_primitives_lss_get_label_name_ids(void* args, void* res);

/**
 * @brief return size of allocated memory for label sets.
 *
 * @param args {
 *     lss uintptr                 // pointer to constructed queryable lss;
 * }
 *
 * @param res {
 *     snapshot          uintptr  // snapshot of lss
 * }
 */
void prompp_create_snapshot_lss(void* args, void* res);

/**
 * @brief Destroy Primitives snapshot LSS.
 *
 * @param args {
 *     snapshot uintptr // pointer of snapshot;
 * }
 */
void prompp_primitives_snapshot_dtor(void* args);

/**
 * @brief returns a copy of the bitset of added series from the lss.
 *
 * @param args {
 *    lss              uintptr  // pointer to constructed queryable lss;
 * }
 *
 * @param res {
 *     bitset          uintptr  // bitset of added series;
 * }
 */
void prompp_primitives_lss_bitset_series(void* args, void* res);

/**
 * @brief destroy bitset of added series.
 *
 * @param args {
 *     bitset          uintptr  // bitset of added series;
 * }
 *
 */
void prompp_primitives_lss_bitset_dtor(void* args);

/**
 * @brief Copy the label sets from the source lss to the destination lss that were added source lss.
 *
 * @param source_snapshot pointer to source snapshot;
 * @param source_bitset pointer to source bitset;
 * @param destination_lss pointer to destination label sets;
 * @param ids_mapping pointer to uintptr
 *
 * @attention This binding used as a CGO call!!!
 *
 */
void prompp_primitives_snapshot_lss_copy_added_series(uint64_t source_snapshot, uint64_t source_bitset, uint64_t destination_lss, uint64_t ids_mapping);

/**
 * @brief destroy ls ids mapping
 *
 * @param args {
 *     ls_ids_mapping uintptr
 * }
 *
 */
void prompp_primitives_free_ls_ids_mapping(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
