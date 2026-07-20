#pragma once

#include "bare_bones/jemalloc.h"
#include "metrics_page.h"

#if JEMALLOC_AVAILABLE

namespace metrics {

struct JemallocMetrics final : MetricsPage<JemallocMetrics> {
  using MetricsPage::MetricsPage;

  CounterRef releases_total{PromPP::Primitives::LabelViewSet{}, "prompp_common_jemalloc_arena_pool_releases_total",
                            &BareBones::jemalloc::FreeArenas::releases_total};
  CounterRef released_bytes_total{PromPP::Primitives::LabelViewSet{}, "prompp_common_jemalloc_arena_pool_released_bytes_total",
                                  &BareBones::jemalloc::FreeArenas::released_bytes_total};
  GaugeRef released_bytes_max{PromPP::Primitives::LabelViewSet{}, "prompp_common_jemalloc_arena_pool_released_bytes_max",
                              &BareBones::jemalloc::FreeArenas::released_bytes_max};
};

}  // namespace metrics

#endif  // JEMALLOC_AVAILABLE
