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
 *        readonly_lss uintptr        // pointer to constructed lss;
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
 *     lss uintptr // pointer to readonly lss
 *     selector uintptr // pointer to constructed selector
 * }
 *
 * @param res {
 *     matches           []uint32 // matched series ids
 *     label_set_lengths []uint16 // slice of series label set length
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

/**
 * @brief return size of allocated memory for label sets.
 *
 * @param args {
 *     lss uintptr                 // pointer to constructed queryable lss;
 * }
 *
 * @param res {
 *     lss_copy          uintptr  // readonly copy of lss
 * }
 */
void prompp_create_readonly_lss(void* args, void* res);

/**
 * @brief Copy label sets which were added via find_or_emplace from source lss to destination lss
 *
 * @param source_lss pointer to source label sets
 * @param destination_lss pointer to destination label sets
 *
 * @attention This binding used as a CGO call!!!
 *
 */
void prompp_primitives_lss_copy_added_series(uint64_t source_lss, uint64_t destination_lss);

#ifdef __cplusplus
}  // extern "C"
#endif
