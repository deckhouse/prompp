#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Initialize metrics iterator
 *
 * @param args *MetricIterator
 */
void prompp_metrics_iterator_ctor(void* args);

/**
 * @brief Serialize metric into protobuf and advance iterator to next metric
 *
 * @param args {
 *   iterator *MetricIterator // Pointer to constructed iterator
 * }
 *
 * @param res {
 *   metric *cppbridge.CppMetric // Pointer to go metric
 * }
 */
void prompp_metrics_iterator_next(void* args, void* res);

/**
 * @brief Create metrics page for test
 *
 * @param args {
 *   labels []cppbridge.Label  // metric page label set
 *   counterName string        // label name for uint64 counter
 *   counterValue uint64       // value for for uint64 counter
 * }
 *
 * @param res {
 *   page uintptr // Pointer to constructed page
 * }
 */
void prompp_metrics_page_for_test_ctor(void* args, void* res);

/**
 * @brief Detach metrics page from storage
 *
 * @param args {
 *   page uintptr // Pointer to constructed page
 * }
 */
void prompp_metrics_page_for_test_detach(void* args);

#ifdef __cplusplus
}
#endif
