#pragma once

#include <algorithm>
#include <cassert>
#include <limits>
#include <string_view>
#include <vector>

#include "bare_bones/vector.h"
#include "parallel_hashmap/phmap.h"
#include "prometheus/tsdb/index/types.h"

namespace series_index::prometheus::tsdb::index {

#pragma pack(push, 1)
template <class Lss>
class IndexWriteContext {
 public:
  struct ExportSymbolId {
    uint8_t source{0};
    uint32_t name_id{std::numeric_limits<uint32_t>::max()};
    uint32_t value_id{std::numeric_limits<uint32_t>::max()};

    bool operator==(const ExportSymbolId&) const noexcept = default;
  };

  struct ExportSymbolIdHasher {
    [[nodiscard]] size_t operator()(const ExportSymbolId& id) const noexcept {
      size_t hash = phmap::phmap_mix<sizeof(size_t)>()(static_cast<size_t>(id.source));
      hash = phmap::phmap_mix<sizeof(size_t)>()(hash + static_cast<size_t>(id.name_id));
      hash = phmap::phmap_mix<sizeof(size_t)>()(hash + static_cast<size_t>(id.value_id));
      return hash;
    }
  };
#pragma pack(pop)

  using SymbolReference = PromPP::Prometheus::tsdb::index::SymbolReference;
  using SymbolReferencesMap = phmap::flat_hash_map<ExportSymbolId, SymbolReference, ExportSymbolIdHasher>;

  explicit IndexWriteContext(const Lss& lss) : lss_(lss) { rebuild(); }
  void rebuild() {
    symbols_.clear();
    symbol_refs_.clear();

    std::vector<ExportSymbolId> symbol_ids;
    symbol_ids.reserve(estimate_symbol_ids_count());
    symbol_ids.emplace_back();

    if (!lss_.is_shrunk_for_export()) [[likely]] {
      const auto view = lss_.data_view();
      for (auto it = view.keys().begin(), e = view.keys().end(); it != e; ++it) {
        symbol_ids.emplace_back(Lss::kSymbolSourceCurrent, it.id(), Lss::kKeyOnlyValueId);
      }
      for (auto it = view.values().begin(), e = view.values().end(); it != e; ++it) {
        symbol_ids.emplace_back(Lss::kSymbolSourceCurrent, it.key_id(), it.value_id());
      }
    } else {
      for (uint32_t ls_id = 0; ls_id < lss_.next_item_index(); ++ls_id) {
        if (lss_.symbol_source_for_series(ls_id) != Lss::kSymbolSourceCurrent) {
          continue;
        }
        const auto labels = lss_[ls_id];
        for (auto label = labels.begin(); label != labels.end(); ++label) {
          symbol_ids.emplace_back(Lss::kSymbolSourceCurrent, label.name_id(), Lss::kKeyOnlyValueId);
          symbol_ids.emplace_back(Lss::kSymbolSourceCurrent, label.name_id(), label.value_id());
        }
      }
    }

    lss_.for_each_snapshot_symbol_id([&](uint32_t name_id, uint32_t value_id) {
      symbol_ids.emplace_back(Lss::kSymbolSourceSnapshot, name_id, Lss::kKeyOnlyValueId);
      symbol_ids.emplace_back(Lss::kSymbolSourceSnapshot, name_id, value_id);
    });

    std::ranges::sort(symbol_ids, [this](const auto& lhs, const auto& rhs) { return resolve_symbol(lhs) < resolve_symbol(rhs); });
    symbols_.reserve(symbol_ids.size());
    symbol_refs_.reserve(symbol_ids.size());

    uint32_t symbol_ref = 0;
    for (auto it = symbol_ids.begin(); it != symbol_ids.end(); ++symbol_ref) {
      symbol_refs_.try_emplace(*it, symbol_ref);
      const auto symbol = resolve_symbol(*it);
      symbols_.emplace_back(symbol);

      while (++it != symbol_ids.end() && symbol == resolve_symbol(*it)) {
        symbol_refs_.try_emplace(*it, symbol_ref);
      }
    }
  }

  template <class Callback>
  void for_each_symbol(Callback&& callback) const {
    for (uint32_t symbol_ref = 0; symbol_ref < symbols_.size(); ++symbol_ref) {
      callback(symbol_ref, symbols_[symbol_ref]);
    }
  }

  [[nodiscard]] SymbolReference symbol_ref_for_name_for_series(uint32_t ls_id, uint32_t name_id) const noexcept {
    return symbol_ref_for_name(symbol_source_for_series(ls_id), name_id);
  }

  [[nodiscard]] SymbolReference symbol_ref_for_value_for_series(uint32_t ls_id, uint32_t name_id, uint32_t value_id) const noexcept {
    return symbol_ref_for_value(symbol_source_for_series(ls_id), name_id, value_id);
  }

  [[nodiscard]] SymbolReference symbol_ref_for_label_index_value(uint32_t name_id, uint32_t value_id) const noexcept {
    const auto current_it = symbol_refs_.find(ExportSymbolId{Lss::kSymbolSourceCurrent, name_id, value_id});
    if (current_it != symbol_refs_.end()) [[likely]] {
      return current_it->second;
    }

    const auto snapshot_it = symbol_refs_.find(ExportSymbolId{Lss::kSymbolSourceSnapshot, name_id, value_id});
    assert(snapshot_it != symbol_refs_.end());
    return snapshot_it->second;
  }

 private:
  const Lss& lss_;
  BareBones::Vector<std::string_view> symbols_;
  SymbolReferencesMap symbol_refs_;

  [[nodiscard]] size_t estimate_symbol_ids_count() const {
    const auto view = lss_.data_view();
    return view.keys().size() + view.values().size();
  }

  [[nodiscard]] uint32_t symbol_source_for_series(uint32_t ls_id) const noexcept { return lss_.symbol_source_for_series(ls_id); }

  [[nodiscard]] SymbolReference symbol_ref_for_name(uint32_t source, uint32_t name_id) const noexcept {
    return symbol_ref_for_id(ExportSymbolId{source, name_id, Lss::kKeyOnlyValueId});
  }

  [[nodiscard]] SymbolReference symbol_ref_for_value(uint32_t source, uint32_t name_id, uint32_t value_id) const noexcept {
    return symbol_ref_for_id(ExportSymbolId{source, name_id, value_id});
  }

  [[nodiscard]] std::string_view resolve_symbol(ExportSymbolId id) const noexcept { return lss_.resolve_symbol_by_source(id.source, id.name_id, id.value_id); }

  [[nodiscard]] SymbolReference symbol_ref_for_id(ExportSymbolId id) const noexcept {
    const auto it = symbol_refs_.find(id);
    assert(it != symbol_refs_.end());
    return it->second;
  }
};

}  // namespace series_index::prometheus::tsdb::index
