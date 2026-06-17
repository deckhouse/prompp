#pragma once

#include <algorithm>
#include <cassert>
#include <limits>
#include <string_view>
#include <vector>

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
    assert(source != SymbolSource::kSnapshot || name_id != kNameIdMask || value_id != kNoId);
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
    // Current-side entries (name symbols + value symbols).
    size_t count = view.keys().size() + view.values().size();
    // Snapshot-side entries (a no-op unless the LSS is shrunk): names once + values.
    lss_.for_each_snapshot_key_id([&](uint32_t) { ++count; });
    lss_.for_each_snapshot_value_id([&](uint32_t, uint32_t) { ++count; });
    return count;
  }

  void collect_current(ExportSymbolIds& symbol_ids) const {
    const auto view = lss_.data_view();
    for (auto it = view.keys().begin(), e = view.keys().end(); it != e; ++it) {
      symbol_ids.emplace_back(SymbolSource::kCurrent, it.id(), kKeyOnlyValueId);
    }
    for (auto it = view.values().begin(), e = view.values().end(); it != e; ++it) {
      symbol_ids.emplace_back(SymbolSource::kCurrent, it.key_id(), it.value_id());
    }
  }

  void collect_snapshot(ExportSymbolIds& symbol_ids) const {
    // Emit each snapshot name symbol once (iterate keys), not once per value.
    lss_.for_each_snapshot_key_id([&](uint32_t name_id) {  //
      symbol_ids.emplace_back(SymbolSource::kSnapshot, name_id, kKeyOnlyValueId);
    });
    lss_.for_each_snapshot_value_id([&](uint32_t name_id, uint32_t value_id) {  //
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
    auto symbol_ids = SymbolIdsCollector<Lss>{lss_}.collect();
    build_symbol_table(symbol_ids);
  }

  template <class Callback>
  void for_each_symbol(Callback&& callback) const {
    uint32_t symbol_ref = 0;
    for (const auto symbol : symbols_) {
      callback(symbol_ref, symbol);
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
  // Unique symbols in output order; string_views point into the LSS (valid for its lifetime).
  BareBones::Vector<std::string_view> symbols_;
  SymbolReferencesMap symbol_refs_;

  void build_symbol_table(const ExportSymbolIds& symbol_ids) {
    // Each symbol is resolved exactly once, grouped in a hash map (O(1) insert),
    // then its keys are sorted once at the end instead of maintaining order per insert.
    phmap::flat_hash_map<std::string_view, std::vector<ExportSymbolId>> symbols_by_string;
    symbols_by_string.reserve(symbol_ids.size());
    for (const auto& symbol_id : symbol_ids) {
      symbols_by_string[resolve_symbol(symbol_id)].emplace_back(symbol_id);
    }

    symbols_.reserve(static_cast<uint32_t>(symbols_by_string.size()));
    for (const auto& [symbol, ids] : symbols_by_string) {
      symbols_.emplace_back(symbol);
    }
    std::ranges::sort(symbols_);

    symbol_refs_.reserve(symbol_ids.size());
    uint32_t symbol_ref = 0;
    for (const auto symbol : symbols_) {
      for (const auto& symbol_id : symbols_by_string.find(symbol)->second) {
        // Same string can be backed by several current and snapshot ids.
        symbol_refs_.try_emplace(symbol_id, symbol_ref);
      }
      ++symbol_ref;
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
