#pragma once

#include "bare_bones/jemalloc.h"
#include "metrics_page.h"

#if JEMALLOC_AVAILABLE

namespace metrics {

struct JemallocMetrics final : MetricsPage<JemallocMetrics> {
  JemallocMetrics() = default;
  explicit JemallocMetrics(uint32_t page_object_size) : MetricsPage(page_object_size) {}
  JemallocMetrics(uint32_t page_object_size, uint32_t metric_offset) : MetricsPage(page_object_size, metric_offset) {}

  template <class MemberType>
    requires std::is_base_of_v<Metric, MemberType>
  explicit JemallocMetrics(MemberType JemallocMetrics::* first_metric_member) : MetricsPage(first_metric_member) {}

  CounterRef releases_total{PromPP::Primitives::LabelViewSet{}, "prompp_common_jemalloc_arena_pool_releases_total",
                            &BareBones::jemalloc::FreeArenas::releases_total};
  CounterRef released_bytes_total{PromPP::Primitives::LabelViewSet{}, "prompp_common_jemalloc_arena_pool_released_bytes_total",
                                  &BareBones::jemalloc::FreeArenas::released_bytes_total};
  GaugeRef released_bytes_max{PromPP::Primitives::LabelViewSet{}, "prompp_common_jemalloc_arena_pool_released_bytes_max",
                              &BareBones::jemalloc::FreeArenas::released_bytes_max};
};

}  // namespace metrics

#endif  // JEMALLOC_AVAILABLE
