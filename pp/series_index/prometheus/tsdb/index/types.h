#pragma once

#include <cstdint>

#include "parallel_hashmap/phmap.h"
#include "primitives/primitives.h"
#include "prometheus/tsdb/index/types.h"

namespace series_index::prometheus::tsdb::index {
using SeriesReferencesMap =
    phmap::flat_hash_map<PromPP::Primitives::LabelSetID, PromPP::Prometheus::tsdb::index::SeriesReference, std::hash<PromPP::Primitives::LabelSetID>>;

struct ChunkMetadata {
  PromPP::Primitives::Timestamp min_timestamp{};
  PromPP::Primitives::Timestamp max_timestamp{};
  uint64_t reference{};
};

}  // namespace series_index::prometheus::tsdb::index