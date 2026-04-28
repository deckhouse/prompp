#pragma once

#include <algorithm>
#include <cassert>
#include <limits>
#include <string_view>
#include <utility>
#include <vector>

#include "bare_bones/vector.h"
#include "parallel_hashmap/phmap.h"
#include "prometheus/tsdb/index/types.h"
#include "series_index/symbol_source.h"

namespace series_index::prometheus::tsdb::index {

template <class Lss>
class IndexWriteContext {
 public:
#pragma pack(push, 1)
  struct ExportSymbolId {
    SymbolSource source{SymbolSource::kCurrent};
    uint32_t name_id{std::numeric_limits<uint32_t>::max()};
    uint32_t value_id{std::numeric_limits<uint32_t>::max()};

    bool operator==(const ExportSymbolId&) const noexcept = default;
  };
#pragma pack(pop)

  struct ExportSymbolIdHasher {
    [[nodiscard]] size_t operator()(const ExportSymbolId& id) const noexcept {
      const uint64_t composite =
          (static_cast<uint64_t>(id.source) << 62U) ^ (static_cast<uint64_t>(id.name_id) << 31U) ^ static_cast<uint64_t>(id.value_id);
      return phmap::phmap_mix<sizeof(size_t)>()(static_cast<size_t>(composite));
    }
  };

  using SymbolReference = PromPP::Prometheus::tsdb::index::SymbolReference;
  using SymbolReferencesMap = phmap::flat_hash_map<ExportSymbolId, SymbolReference, ExportSymbolIdHasher>;
  using SymbolIdWithView = std::pair<std::string_view, ExportSymbolId>;

  explicit IndexWriteContext(const Lss& lss) : lss_(lss) { rebuild(); }

  void rebuild() {
    symbols_.clear();
    symbol_refs_.clear();

    std::vector<SymbolIdWithView> symbol_ids;
    symbol_ids.reserve(estimate_symbol_ids_count());
    collect_empty_symbol(symbol_ids);
    collect_current_symbols(symbol_ids);
    collect_snapshot_symbols(symbol_ids);
    build_symbol_table(symbol_ids);
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
    const auto current_it = symbol_refs_.find(ExportSymbolId{SymbolSource::kCurrent, name_id, value_id});
    if (current_it != symbol_refs_.end()) {
      return current_it->second;
    }

    const auto snapshot_it = symbol_refs_.find(ExportSymbolId{SymbolSource::kSnapshot, name_id, value_id});
    assert(snapshot_it != symbol_refs_.end());
    return snapshot_it->second;
  }

 private:
  const Lss& lss_;
  BareBones::Vector<std::string_view> symbols_;
  SymbolReferencesMap symbol_refs_;

  void collect_empty_symbol(std::vector<SymbolIdWithView>& symbol_ids) const { symbol_ids.emplace_back(std::string_view{}, ExportSymbolId{}); }

  void collect_current_symbols(std::vector<SymbolIdWithView>& symbol_ids) const {
    if (!lss_.shrink_state().is_shrunk()) {
      collect_current_symbols_from_bimap(symbol_ids);
      return;
    }
    collect_current_symbols_from_shrunk_series(symbol_ids);
  }

  void collect_current_symbols_from_bimap(std::vector<SymbolIdWithView>& symbol_ids) const {
    const auto view = lss_.data_view();
    for (auto it = view.keys().begin(), e = view.keys().end(); it != e; ++it) {
      symbol_ids.emplace_back(view.key_symbol(it.id()), ExportSymbolId{SymbolSource::kCurrent, it.id(), kKeyOnlyValueId});
    }
    for (auto it = view.values().begin(), e = view.values().end(); it != e; ++it) {
      symbol_ids.emplace_back(view.value_symbol(it.key_id(), it.value_id()), ExportSymbolId{SymbolSource::kCurrent, it.key_id(), it.value_id()});
    }
  }

  void collect_current_symbols_from_shrunk_series(std::vector<SymbolIdWithView>& symbol_ids) const {
    for (uint32_t ls_id = lss_.shrink_state().shift; ls_id < lss_.max_item_index(); ++ls_id) {
      if (lss_.symbol_source_for_series(ls_id) != SymbolSource::kCurrent) {
        continue;
      }
      const auto labels = lss_[ls_id];
      for (auto label = labels.begin(); label != labels.end(); ++label) {
        emplace_resolved_symbol(symbol_ids, SymbolSource::kCurrent, label.name_id(), kKeyOnlyValueId);
        emplace_resolved_symbol(symbol_ids, SymbolSource::kCurrent, label.name_id(), label.value_id());
      }
    }
  }

  void collect_snapshot_symbols(std::vector<SymbolIdWithView>& symbol_ids) const {
    lss_.for_each_snapshot_symbol_id([&](uint32_t name_id, uint32_t value_id) {
      emplace_resolved_symbol(symbol_ids, SymbolSource::kSnapshot, name_id, kKeyOnlyValueId);
      emplace_resolved_symbol(symbol_ids, SymbolSource::kSnapshot, name_id, value_id);
    });
  }

  void emplace_resolved_symbol(std::vector<SymbolIdWithView>& symbol_ids, SymbolSource source, uint32_t name_id, uint32_t value_id) const {
    symbol_ids.emplace_back(lss_.resolve_symbol_by_source(source, name_id, value_id), ExportSymbolId{source, name_id, value_id});
  }

  void build_symbol_table(std::vector<SymbolIdWithView>& symbol_ids) {
    std::ranges::sort(symbol_ids, [](const auto& lhs, const auto& rhs) { return lhs.first < rhs.first; });
    symbols_.reserve(symbol_ids.size());
    symbol_refs_.reserve(symbol_ids.size());

    uint32_t symbol_ref = 0;
    for (auto it = symbol_ids.begin(); it != symbol_ids.end(); ++symbol_ref) {
      symbol_refs_.try_emplace(it->second, symbol_ref);
      const auto symbol = it->first;
      symbols_.emplace_back(symbol);

      auto next = it;
      while (true) {
        ++next;
        if (next == symbol_ids.end() || symbol != next->first) {
          break;
        }
        symbol_refs_.try_emplace(next->second, symbol_ref);
      }
      it = next;
    }
  }

  [[nodiscard]] size_t estimate_symbol_ids_count() const {
    const auto view = lss_.data_view();
    if (!lss_.shrink_state().is_shrunk()) {
      return 1 + view.keys().size() + view.values().size();
    }
    return 1 + std::max(view.keys().size() + view.values().size(), static_cast<size_t>(lss_.series_count()) * 2U);
  }

  [[nodiscard]] SymbolSource symbol_source_for_series(uint32_t ls_id) const noexcept { return lss_.symbol_source_for_series(ls_id); }

  [[nodiscard]] SymbolReference symbol_ref_for_name(SymbolSource source, uint32_t name_id) const noexcept {
    return symbol_ref_for_id(ExportSymbolId{source, name_id, kKeyOnlyValueId});
  }

  [[nodiscard]] SymbolReference symbol_ref_for_value(SymbolSource source, uint32_t name_id, uint32_t value_id) const noexcept {
    return symbol_ref_for_id(ExportSymbolId{source, name_id, value_id});
  }

  [[nodiscard]] SymbolReference symbol_ref_for_id(ExportSymbolId id) const noexcept {
    const auto it = symbol_refs_.find(id);
    assert(it != symbol_refs_.end());
    return it->second;
  }
};

}  // namespace series_index::prometheus::tsdb::index
