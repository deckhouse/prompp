#pragma once

#include "metrics_page_list.h"

namespace metrics {

inline MetricsPageList storage;

template <class MetricsPageType, class LabelSet>
PROMPP_ALWAYS_INLINE MetricsPageType* CreateMetricsPage(LabelSet&& label_set) {
  auto* page = new MetricsPageType(std::forward<LabelSet>(label_set));
  storage.add(page);
  return page;
}

}  // namespace metrics