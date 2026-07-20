#pragma once

#include <cstdint>

#include "bare_bones/vector.h"
#include "primitives/primitives.h"
#include "prometheus/tsdb/index/types.h"

namespace series_index::prometheus::tsdb::index {

// Series references are indexed densely by LabelSetID. A reference is the series
// offset in the produced index file divided by kSeriesAlignment; since the symbols
// table is always written before the series section, a written series reference is
// never zero. Zero is therefore used as a sentinel for a series that has not been
// written and must be skipped.
inline constexpr PromPP::Prometheus::tsdb::index::SeriesReference kUnwrittenSeriesReference = 0;

using SeriesReferences = BareBones::Vector<PromPP::Prometheus::tsdb::index::SeriesReference>;

struct ChunkMetadata {
  PromPP::Primitives::Timestamp min_timestamp{};
  PromPP::Primitives::Timestamp max_timestamp{};
  uint64_t reference{};
};

}  // namespace series_index::prometheus::tsdb::index
