#include "metrics.h"

#include <memory>

#include "metrics/counter.h"
#include "metrics/serializer.h"
#include "metrics/storage.h"
#include "primitives/go_model.h"
#include "primitives/go_slice.h"

struct MetricsIterator {
  metrics::Storage::Iterator iterator{metrics::storage().begin()};
  metrics::Serializer serializer;
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

extern "C" void prompp_metrics_iterator_serialize(void* args, void* res) {
  struct Arguments {
    MetricsIteratorPtr iterator;
  };
  struct Result {
    SliceView<const char> buffer;
  };

  const auto it = static_cast<Arguments*>(args)->iterator.get();
  const auto out = static_cast<Result*>(res);

  if (it->iterator == metrics::Storage::end()) [[unlikely]] {
    out->buffer.reset_to(nullptr, 0, 0);
  } else {
    const auto& buffer = it->serializer.serialize(it->iterator->page->labels(), it->iterator->metric);
    out->buffer.reset_to(buffer.c_str(), buffer.size(), buffer.capacity());
    ++it->iterator;
  }
}

struct MetricsPageForTest final : metrics::MetricsPage<MetricsPageForTest> {
  using MetricsPage::MetricsPage;

  MetricsPageForTest(PromPP::Primitives::LabelViewSet&& label_set, const String& counter_name, uint64_t counter_value)
      : MetricsPage(std::move(label_set)), emplace_count(static_cast<std::string_view>(counter_name), counter_value) {}

  metrics::Counter<> emplace_count;
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

  PromPP::Primitives::LabelViewSet label_set;
  label_set.append_unsorted(in->labels);

  new (res) Result{
      .page = metrics::CreateMetricsPage<MetricsPageForTest>(std::move(label_set), in->counter_name, in->counter_value),
  };
}

extern "C" void prompp_metrics_page_for_test_detach(void* args) {
  struct Arguments {
    MetricsPageForTest* page;
  };

  static_cast<Arguments*>(args)->page->detach();
}
