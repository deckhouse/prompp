#pragma once

#include <span>

#include <parallel_hashmap/phmap.h>

#include "bare_bones/vector.h"
#include "bare_bones/xxhash.h"

namespace series_index::querier {

template <class Lss, template <class> class Vector>
void group_series_by_label_names(const Lss& lss,
                                 std::span<const uint32_t> series_ids,
                                 std::span<const uint32_t> label_name_ids,
                                 Vector<Vector<uint32_t>>& groups) {
  struct SetItem {
    BareBones::Vector<uint32_t> values_ids;
    uint32_t group_index;
  };

  class HashCalculator {
   public:
    PRAGMA_DIAGNOSTIC(push)
    PRAGMA_DIAGNOSTIC(ignored "-Wunused-local-typedefs")
    using is_transparent = void;
    PRAGMA_DIAGNOSTIC(pop)

    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t operator()(const SetItem& key) const noexcept {
      return BareBones::XXHash3::hash(key.values_ids.data(), key.values_ids.size());
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t operator()(const BareBones::Vector<uint32_t>& values_ids) const noexcept {
      return BareBones::XXHash3::hash(std::span(values_ids));
    }
  };

  class EqualTo {
   public:
    PRAGMA_DIAGNOSTIC(push)
    PRAGMA_DIAGNOSTIC(ignored "-Wunused-local-typedefs")
    using is_transparent = void;
    PRAGMA_DIAGNOSTIC(pop)

    PROMPP_ALWAYS_INLINE bool operator()(const SetItem& a, const SetItem& b) const noexcept { return a.values_ids == b.values_ids; }
    PROMPP_ALWAYS_INLINE bool operator()(const SetItem& a, const BareBones::Vector<uint32_t>& b) const noexcept { return a.values_ids == b; }
  };

  static constexpr auto kInvalidLabelValueId = std::numeric_limits<uint32_t>::max();

  phmap::flat_hash_set<SetItem, HashCalculator, EqualTo> groups_set;

  BareBones::Vector<uint32_t> values_ids(label_name_ids.size());

  for (const auto series_id : series_ids) {
    std::ranges::fill(values_ids, kInvalidLabelValueId);

    const auto label_set = lss[series_id];
    for (auto it = label_set.begin(); it != label_set.end(); ++it) {
      if (const auto index = std::ranges::find(label_name_ids, it.name_id()); index != label_name_ids.end()) {
        values_ids[index - label_name_ids.begin()] = it.value_id();
      }
    }

    const auto it = groups_set.lazy_emplace(values_ids, [&](const auto& ctor) {
      ctor(SetItem{.values_ids = std::move(values_ids), .group_index = static_cast<uint32_t>(groups.size())});
      groups.emplace_back();
      values_ids.resize(label_name_ids.size());
    });
    groups[it->group_index].emplace_back(series_id);
  }
}

}  // namespace series_index::querier