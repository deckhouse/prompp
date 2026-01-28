#include "metrics.h"

#include "metrics/storage.h"
#include "primitives/go_model.h"
#include "primitives/go_slice.h"

#include "metrics/global_metrics.h"

using PromPP::Primitives::Go::Label;
using PromPP::Primitives::Go::SliceView;
using PromPP::Primitives::Go::String;

extern "C" void prompp_metrics_iterator_ctor(void* args) {
  metrics::storage().remove_unused_pages();

  metrics::global_metrics()->test_counter.inc(1);

  std::construct_at(static_cast<metrics::Storage::Iterator*>(args), metrics::storage().begin());
}

extern "C" void prompp_metrics_iterator_next(void* args, void* res) {
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

extern "C" void prompp_metrics_page_for_test_ctor(void* args, void* res) {
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

extern "C" void prompp_metrics_page_for_test_detach(void* args) {
  struct Arguments {
    MetricsPageForTest* page;
  };

  static_cast<Arguments*>(args)->page->detach();
}
