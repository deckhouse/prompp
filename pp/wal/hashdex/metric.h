#pragma once

#include "primitives/histogram.h"
#include "primitives/timeseries.h"
#include "prometheus/metric.h"

namespace PromPP::WAL::hashdex {

struct FloatMetric {
  Primitives::TimeseriesSemiview timeseries{};
  uint64_t hash{};

  bool operator==(const FloatMetric&) const noexcept = default;
};

struct HistogramMetric : Primitives::BasicHistogram<Primitives::LabelViewSet, BareBones::Vector, BareBones::Vector, BareBones::Vector> {
  uint64_t hash{};

  bool operator==(const HistogramMetric&) const noexcept = default;
};

#pragma pack(push, 1)

struct Metadata {
  std::string_view metric_name{};
  std::string_view text{};
  Prometheus::MetadataType type{};

  bool operator==(const Metadata&) const noexcept = default;
};

#pragma pack(pop)

}  // namespace PromPP::WAL::hashdex