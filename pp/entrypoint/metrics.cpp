#include "metrics.h"

#include <memory>

#include "metrics/counter.h"
#include "metrics/storage.h"
#include "primitives/go_model.h"
#include "primitives/go_slice.h"

struct MetricsIterator {
  metrics::Storage::Iterator iterator{metrics::storage().begin()};
};

using MetricsIteratorPtr = std::unique_ptr<MetricsIterator>;

using PromPP::Primitives::Go::Label;
using PromPP::Primitives::Go::SliceView;
using PromPP::Primitives::Go::String;

extern "C" void prompp_metrics_iterator_ctor(void* res) {
  struct Result {
    MetricsIteratorPtr iterator;
  };

  metrics::storage().remove_unused_pages();

  new (res) Result{.iterator = std::make_unique<MetricsIterator>()};
}

extern "C" void prompp_metrics_iterator_dtor(void* args) {
  struct Arguments {
    MetricsIteratorPtr iterator;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_metrics_iterator_next(void* args, void* res) {
  struct Arguments {
    MetricsIteratorPtr iterator;
  };
  struct Result {
    const PromPP::Primitives::Go::Metric* metric;
  };

  const auto it = static_cast<Arguments*>(args)->iterator.get();
  const auto out = static_cast<Result*>(res);

  if (it->iterator == metrics::Storage::end()) [[unlikely]] {
    out->metric = nullptr;
  } else {
    out->metric = it->iterator->metric->go_metric();
    ++it->iterator;
  }
}

struct MetricsPageForTest final : metrics::MetricsPage<MetricsPageForTest> {
  using MetricsPage::MetricsPage;

  MetricsPageForTest(const SliceView<Label>& labels, const String& counter_name, uint64_t counter_value)
      : emplace_count(labels, static_cast<std::string_view>(counter_name), counter_value) {}

  metrics::Counter emplace_count;
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
