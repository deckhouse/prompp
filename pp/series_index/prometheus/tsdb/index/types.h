#pragma once

#include <cstdint>
#include <limits>
#include <vector>

#include "bare_bones/preprocess.h"
#include "parallel_hashmap/phmap.h"
#include "primitives/primitives.h"
#include "prometheus/tsdb/index/types.h"

namespace series_index::prometheus::tsdb::index {

struct SymbolLssIdWithSource {
  static constexpr uint32_t kSourceCurrent = 0;
  static constexpr uint32_t kSourceSnapshot = 1;
  /** Sentinel: value_id == kNoId means key (name) symbol; name_id and value_id both kNoId means empty. */
  static constexpr uint32_t kNoId = std::numeric_limits<uint32_t>::max();

  uint32_t source{kSourceCurrent};
  uint32_t name_id{kNoId};
  uint32_t value_id{kNoId};

  constexpr SymbolLssIdWithSource() = default;
  constexpr SymbolLssIdWithSource(uint32_t _source, uint32_t _name_id, uint32_t _value_id) : source(_source), name_id(_name_id), value_id(_value_id) {}

  bool operator==(const SymbolLssIdWithSource&) const noexcept = default;

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_empty() const noexcept { return name_id == kNoId && value_id == kNoId; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_name() const noexcept { return value_id == kNoId; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_duplicated() const noexcept { return name_id == kNoId && value_id == 0; }
  PROMPP_ALWAYS_INLINE void mark_as_duplicated() noexcept {
    name_id = kNoId;
    value_id = 0;
  }
};

using SymbolReferencesMap = phmap::flat_hash_map<SymbolLssIdWithSource, PromPP::Prometheus::tsdb::index::SymbolReference, std::hash<SymbolLssIdWithSource>>;
using SeriesReferencesMap =
    phmap::flat_hash_map<PromPP::Primitives::LabelSetID, PromPP::Prometheus::tsdb::index::SeriesReference, std::hash<PromPP::Primitives::LabelSetID>>;

struct ChunkMetadata {
  PromPP::Primitives::Timestamp min_timestamp{};
  PromPP::Primitives::Timestamp max_timestamp{};
  uint64_t reference{};
};

}  // namespace series_index::prometheus::tsdb::index

template <>
struct std::hash<series_index::prometheus::tsdb::index::SymbolLssIdWithSource> {
  PROMPP_ALWAYS_INLINE size_t operator()(const series_index::prometheus::tsdb::index::SymbolLssIdWithSource& id) const noexcept {
    size_t h = phmap::phmap_mix<sizeof(size_t)>()(static_cast<size_t>(id.source));
    h = phmap::phmap_mix<sizeof(size_t)>()(h + static_cast<size_t>(id.name_id));
    h = phmap::phmap_mix<sizeof(size_t)>()(h + static_cast<size_t>(id.value_id));
    return h;
  }
};