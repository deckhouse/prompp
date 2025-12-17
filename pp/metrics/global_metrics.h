#pragma once

#include "metrics/calculator.h"
#include "metrics/counter.h"
#include "metrics/storage.h"

namespace metrics {

struct GlobalMetrics final : MetricsPage<GlobalMetrics> {
  using MetricsPage::MetricsPage;

  Counter data_storage_allocations{PromPP::Primitives::LabelViewSet{}, "prompp_data_storage_allocations"};
  Counter lss_allocations{PromPP::Primitives::LabelViewSet{}, "prompp_lss_allocations"};
};

PROMPP_ALWAYS_INLINE GlobalMetrics* global_metrics() {
  static auto metrics = ::metrics::CreateMetricsPage<GlobalMetrics>();
  return metrics;
}

}  // namespace metrics