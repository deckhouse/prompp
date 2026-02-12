#pragma once

#include "bare_bones/memory.h"
#include "metrics/storage.h"

namespace metrics {

struct MallocMetrics final : MetricsPage<MallocMetrics> {
  using MetricsPage::MetricsPage;

  AtomicCounterFromPtr malloc_count{PromPP::Primitives::LabelViewSet{}, "prompp_cpp_malloc_count", &BareBones::malloc_count};
  AtomicCounterFromPtr realloc_count{PromPP::Primitives::LabelViewSet{}, "prompp_cpp_realloc_count", &BareBones::realloc_count};
  AtomicCounterFromPtr realloc_grow_count{PromPP::Primitives::LabelViewSet{}, "prompp_cpp_realloc_grow_count", &BareBones::realloc_grow_count};
};

PROMPP_ALWAYS_INLINE MallocMetrics* global_metrics() {
  static auto metrics = ::metrics::CreateMetricsPage<MallocMetrics>();
  return metrics;
}

inline const auto kRegisteredMallocMetrics = global_metrics();

}  // namespace metrics