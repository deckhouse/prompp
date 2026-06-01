#pragma once

#include <algorithm>
#include <cassert>
#include <limits>
#include <string_view>

#include "bare_bones/vector.h"
#include "parallel_hashmap/phmap.h"
#include "prometheus/tsdb/index/types.h"
#include "series_index/symbol_source.h"

namespace series_index::prometheus::tsdb::index {

struct ExportSymbolId {
  static constexpr uint32_t kSourceBitShift = 31U;
  static constexpr uint32_t kNameIdMask = (1U << kSourceBitShift) - 1U;
  static constexpr uint32_t kNoId = std::numeric_limits<uint32_t>::max();

  uint32_t name_id_bits : 31 {kNameIdMask};
  uint32_t source_bit : 1 {1U};
  uint32_t value_id{std::numeric_limits<uint32_t>::max()};

  constexpr ExportSymbolId() = default;
  constexpr ExportSymbolId(SymbolSource source, uint32_t name_id, uint32_t value_id)
      : name_id_bits(name_id), source_bit(pack_source(source)), value_id(value_id) {
    assert(name_id <= kNameIdMask);
  }

  bool operator==(const ExportSymbolId&) const noexcept = default;

  [[nodiscard]] constexpr bool is_empty() const noexcept { return source_bit == 1U && name_id_bits == kNameIdMask && value_id == kNoId; }
  [[nodiscard]] constexpr SymbolSource source() const noexcept { return source_bit == 0U ? SymbolSource::kCurrent : SymbolSource::kSnapshot; }
  [[nodiscard]] constexpr uint32_t name_id() const noexcept { return name_id_bits; }
  [[nodiscard]] constexpr uint32_t packed_name_id() const noexcept { return (source_bit << kSourceBitShift) | name_id_bits; }

 private:
  [[nodiscard]] static constexpr uint32_t pack_source(SymbolSource source) noexcept { return source == SymbolSource::kSnapshot ? 1U : 0U; }
};

struct ExportSymbolIdHasher {
  [[nodiscard]] size_t operator()(const ExportSymbolId& id) const noexcept {
    size_t hash = phmap::phmap_mix<sizeof(size_t)>()(static_cast<size_t>(id.packed_name_id()));
    hash = phmap::phmap_mix<sizeof(size_t)>()(hash + static_cast<size_t>(id.value_id));
    return hash;
  }
};

using ExportSymbolIds = BareBones::Vector<ExportSymbolId>;

template <class Lss>
class SymbolIdsCollector {
 public:
  explicit SymbolIdsCollector(const Lss& lss) : lss_(lss) {}

  [[nodiscard]] ExportSymbolIds collect() const {
    ExportSymbolIds symbol_ids;
    symbol_ids.reserve(static_cast<uint32_t>(estimate_count()));
    symbol_ids.emplace_back();
    collect_current(symbol_ids);
    collect_snapshot(symbol_ids);
    return symbol_ids;
  }

 private:
  const Lss& lss_;

  [[nodiscard]] size_t estimate_count() const {
    const auto view = lss_.data_view();
    if (!lss_.shrink_state().is_shrunk()) {
      return 1 + view.keys().size() + view.values().size();
    }
    return 1 + std::max(view.keys().size() + view.values().size(), static_cast<size_t>(lss_.items_count()) * 2U);
  }

  void collect_current(ExportSymbolIds& symbol_ids) const {
    if (!lss_.shrink_state().is_shrunk()) {
      collect_current_from_bimap(symbol_ids);
      return;
    }
    collect_current_from_shrunk_series(symbol_ids);
  }

  void collect_current_from_bimap(ExportSymbolIds& symbol_ids) const {
    const auto view = lss_.data_view();
    for (auto it = view.keys().begin(), e = view.keys().end(); it != e; ++it) {
      symbol_ids.emplace_back(SymbolSource::kCurrent, it.id(), kKeyOnlyValueId);
    }
    for (auto it = view.values().begin(), e = view.values().end(); it != e; ++it) {
      symbol_ids.emplace_back(SymbolSource::kCurrent, it.key_id(), it.value_id());
    }
  }

  void collect_current_from_shrunk_series(ExportSymbolIds& symbol_ids) const {
    for (uint32_t ls_id = lss_.shrink_state().shift; ls_id < lss_.next_item_index(); ++ls_id) {
      const auto labels = lss_[ls_id];
      for (auto label = labels.begin(); label != labels.end(); ++label) {
        symbol_ids.emplace_back(SymbolSource::kCurrent, label.name_id(), kKeyOnlyValueId);
        symbol_ids.emplace_back(SymbolSource::kCurrent, label.name_id(), label.value_id());
      }
    }
  }

  void collect_snapshot(ExportSymbolIds& symbol_ids) const {
    lss_.for_each_snapshot_symbol_id([&](uint32_t name_id, uint32_t value_id) {
      symbol_ids.emplace_back(SymbolSource::kSnapshot, name_id, kKeyOnlyValueId);
      symbol_ids.emplace_back(SymbolSource::kSnapshot, name_id, value_id);
    });
  }
};

template <class Lss>
class IndexWriteContext {
 public:
  using SymbolReference = PromPP::Prometheus::tsdb::index::SymbolReference;
  using SymbolReferencesMap = phmap::flat_hash_map<ExportSymbolId, SymbolReference, ExportSymbolIdHasher>;

  explicit IndexWriteContext(const Lss& lss) : lss_(lss) { rebuild(); }

  void rebuild() {
    symbols_.clear();
    symbol_refs_.clear();
    build_symbol_table(SymbolIdsCollector<Lss>{lss_}.collect());
  }

  template <class Callback>
  void for_each_symbol(Callback&& callback) const {
    uint32_t symbol_ref = 0;
    for (const auto& symbol_id : symbols_) {
      callback(symbol_ref, resolve_symbol(symbol_id));
      ++symbol_ref;
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
  ExportSymbolIds symbols_;
  SymbolReferencesMap symbol_refs_;

  void build_symbol_table(ExportSymbolIds symbol_ids) {
    std::ranges::sort(symbol_ids, [this](const auto& lhs, const auto& rhs) { return resolve_symbol(lhs) < resolve_symbol(rhs); });
    symbols_.reserve(symbol_ids.size());
    symbol_refs_.reserve(symbol_ids.size());

    uint32_t symbol_ref = 0;
    for (auto it = symbol_ids.begin(); it != symbol_ids.end(); ++symbol_ref) {
      symbol_refs_.try_emplace(*it, symbol_ref);
      const auto symbol = resolve_symbol(*it);
      symbols_.emplace_back(*it);

      auto next = it;
      while (true) {
        ++next;
        if (next == symbol_ids.end() || symbol != resolve_symbol(*next)) {
          break;
        }
        symbol_refs_.try_emplace(*next, symbol_ref);
      }
      it = next;
    }
  }

  [[nodiscard]] SymbolSource symbol_source_for_series(uint32_t ls_id) const noexcept { return lss_.symbol_source_for_series(ls_id); }

  [[nodiscard]] SymbolReference symbol_ref_for_name(SymbolSource source, uint32_t name_id) const noexcept {
    return symbol_ref_for_id(ExportSymbolId{source, name_id, kKeyOnlyValueId});
  }

  [[nodiscard]] SymbolReference symbol_ref_for_value(SymbolSource source, uint32_t name_id, uint32_t value_id) const noexcept {
    return symbol_ref_for_id(ExportSymbolId{source, name_id, value_id});
  }

  [[nodiscard]] std::string_view resolve_symbol(ExportSymbolId id) const noexcept {
    if (id.is_empty()) {
      return {};
    }
    return lss_.resolve_symbol_by_source(id.source(), id.name_id(), id.value_id);
  }

  [[nodiscard]] SymbolReference symbol_ref_for_id(ExportSymbolId id) const noexcept {
    const auto it = symbol_refs_.find(id);
    assert(it != symbol_refs_.end());
    return it->second;
  }
};

}  // namespace series_index::prometheus::tsdb::index
