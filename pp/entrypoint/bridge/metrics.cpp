#include "metrics.h"
#include "annotations.h"

#include "metrics/jemalloc_metrics.h"
#include "metrics/storage.h"
#include "primitives/go_model.h"
#include "primitives/go_slice.h"

using PromPP::Primitives::Go::Label;
using PromPP::Primitives::Go::SliceView;
using PromPP::Primitives::Go::String;

/**
 * @brief Register cpp metrics
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_metrics_register() {
  [[maybe_unused]] static auto _ = [] {
#if JEMALLOC_AVAILABLE
    metrics::CreateMetricsPage<metrics::JemallocMetrics>();
#endif
    return 0;
  }();
}

/**
 * @brief Initialize metrics iterator
 *
 * @param args *MetricIterator
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_metrics_iterator_ctor(void* args) {
  using Arguments = metrics::Storage::Iterator;

  metrics::storage.remove_unused_pages();

  std::construct_at(static_cast<Arguments*>(args), metrics::storage.begin());
}

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
extern "C" PROMPP(entrypoint, fastcgo) void prompp_metrics_iterator_next(void* args, void* res) {
  struct Arguments {
    metrics::Storage::Iterator* iterator;
  };
  struct Result {
    const PromPP::Primitives::Go::Metric* metric;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = static_cast<Result*>(res);

  if (*in->iterator == metrics::Storage::end()) [[unlikely]] {
    out->metric = nullptr;
  } else {
    out->metric = (*in->iterator)->go_metric();
    ++(*in->iterator);
  }
}

struct MetricsPageForTest final : metrics::MetricsPage<MetricsPageForTest> {
  using MetricsPage::MetricsPage;

  MetricsPageForTest(const SliceView<Label>& labels, const String& counter_name, uint64_t counter_value)
      : emplace_count(labels, static_cast<std::string_view>(counter_name), counter_value),
        emplace_gauge(labels, static_cast<std::string_view>(counter_name), counter_value) {}

  metrics::Counter emplace_count;
  metrics::Gauge emplace_gauge;
};

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
extern "C" PROMPP(entrypoint, fastcgo) void prompp_metrics_page_for_test_ctor(void* args, void* res) {
  struct Arguments {
    SliceView<Label> labels;
    String counter_name;
    uint64_t counter_value;
  };
  struct Result {
    MetricsPageForTest* page;
  };

  const auto in = static_cast<Arguments*>(args);

  new (res) Result{
      .page = metrics::CreateMetricsPage<MetricsPageForTest>(in->labels, in->counter_name, in->counter_value),
  };
}

/**
 * @brief Detach metrics page from storage
 *
 * @param args {
 *   page uintptr // Pointer to constructed page
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_metrics_page_for_test_detach(void* args) {
  struct Arguments {
    MetricsPageForTest* page;
  };

  static_cast<Arguments*>(args)->page->detach();
}
