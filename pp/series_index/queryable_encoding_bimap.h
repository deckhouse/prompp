#pragma once

#include <ranges>

#include "bare_bones/allocator.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/snug_composite.h"
#include "queried_series.h"
#include "reverse_index.h"
#include "sorting_index.h"
#include "trie_index.h"

namespace series_index {

template <template <template <class> class> class Filament, template <class> class Vector, class TrieIndex, class ReverseIndexType>
class QueryableEncodingBimap final
    : public BareBones::SnugComposite::GenericDecodingTable<QueryableEncodingBimap<Filament, Vector, TrieIndex, ReverseIndexType>, Filament, Vector> {
 public:
  using Base = BareBones::SnugComposite::GenericDecodingTable<QueryableEncodingBimap, Filament, Vector>;
  using LsIdSet = phmap::btree_set<typename Base::Proxy, typename Base::LessComparator, BareBones::Allocator<typename Base::Proxy>>;
  using HashSet =
      phmap::flat_hash_set<typename Base::Proxy, typename Base::Hasher, typename Base::EqualityComparator, BareBones::Allocator<typename Base::Proxy>>;
  using LsIdSetIterator = typename LsIdSet::const_iterator;
  using TrieIndexIterator = typename TrieIndex::Iterator;
  using ReverseIndex = ReverseIndexType;

  using Base::reserve;

  friend class BareBones::SnugComposite::GenericDecodingTable<QueryableEncodingBimap, Filament, Vector>;

  [[nodiscard]] PROMPP_ALWAYS_INLINE const TrieIndex& trie_index() const noexcept { return trie_index_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const ReverseIndex& reverse_index() const noexcept { return reverse_index_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const LsIdSet& ls_id_set() const noexcept { return ls_id_set_; }

  template <class Iterator>
  PROMPP_ALWAYS_INLINE void sort_series_ids(Iterator begin, Iterator end) noexcept {
    sorting_index_.sort(begin, end);
  }

  template <class Container>
  PROMPP_ALWAYS_INLINE void sort_series_ids(Container& container) noexcept {
    sort_series_ids(container.begin(), container.end());
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    return trie_index_.allocated_memory() + reverse_index_.allocated_memory() + ls_id_set_allocated_memory_ + ls_id_hash_set_allocated_memory_ +
           sorting_index_.allocated_memory() + queried_series_.allocated_memory() + Base::allocated_memory();
  }

  template <class LabelSet>
  PROMPP_ALWAYS_INLINE uint32_t find_or_emplace(const LabelSet& label_set) noexcept {
    return find_or_emplace(label_set, Base::hasher()(label_set));
  }

  template <class LabelSet>
  PROMPP_ALWAYS_INLINE uint32_t find_or_emplace(const LabelSet& label_set, size_t hash) noexcept {
    hash = phmap_hash(hash);
    if (auto it = ls_id_hash_set_.find(label_set, hash); it != ls_id_hash_set_.end()) {
      mark_series_as_added(*it);
      return *it;
    }

    auto ls_id = Base::items_.size();
    auto composite_label_set = Base::items_.emplace_back(Base::data_, label_set).composite(Base::data());
    update_indexes(ls_id, composite_label_set, hash);
    queried_series_.set_series_count(Base::items_.size());
    mark_series_as_added(ls_id);
    return ls_id;
  }

  template <class Class>
  PROMPP_ALWAYS_INLINE std::optional<uint32_t> find(const Class& c) const noexcept {
    return find(c, Base::hasher()(c));
  }

  template <class Class>
  PROMPP_ALWAYS_INLINE std::optional<uint32_t> find(const Class& c, size_t hashval) const noexcept {
    if (auto i = ls_id_hash_set_.find(c, phmap_hash(hashval)); i != ls_id_hash_set_.end()) {
      return *i;
    }
    return {};
  }

  template <class SeriesIdContainer>
  PROMPP_ALWAYS_INLINE void set_queried_series(QueriedSeries::Source source, const SeriesIdContainer& ids) noexcept {
    queried_series_.set(source, ids);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t queried_series_count(QueriedSeries::Source source) const noexcept { return queried_series_.count(source); }

  PROMPP_ALWAYS_INLINE void reserve(uint32_t count) {
    Base::items_.reserve(count);
    ls_id_hash_set_.reserve(count);
    queried_series_.reserve(count);
    added_series_.reserve(count);
  }

 private:
  using LabelSet = typename Base::value_type;

  template <class AnyQueryableEncodingBimap>
  friend class QueryableEncodingBimapCopier;

  TrieIndex trie_index_;
  ReverseIndex reverse_index_;

  size_t ls_id_set_allocated_memory_{};
  bool ls_id_comparator_enabled_{true};
  LsIdSet ls_id_set_{{}, Base::less_comparator(&ls_id_comparator_enabled_), BareBones::Allocator<typename Base::Proxy>{ls_id_set_allocated_memory_}};

  size_t ls_id_hash_set_allocated_memory_{};
  HashSet ls_id_hash_set_{0, Base::hasher(), Base::equality_comparator(), BareBones::Allocator<typename Base::Proxy>{ls_id_hash_set_allocated_memory_}};

  SortingIndex<LsIdSet> sorting_index_{ls_id_set_};

  QueriedSeries queried_series_;

  BareBones::Bitset added_series_;

  PROMPP_ALWAYS_INLINE void after_items_load_impl(uint32_t first_loaded_id) noexcept {
    ls_id_hash_set_.reserve(Base::items_.size());
    queried_series_.set_series_count(Base::items_.size());

    const auto hasher = Base::hasher();
    for (auto ls_id = first_loaded_id; ls_id < Base::items_.size(); ++ls_id) {
      auto label_set = this->operator[](ls_id);
      update_indexes(ls_id, label_set, phmap_hash(hasher(label_set)));
    }
  }

  void update_indexes(uint32_t ls_id, const LabelSet& label_set, size_t label_set_phmap_hash) {
    ls_id_hash_set_.emplace_with_hash(label_set_phmap_hash, typename Base::Proxy(ls_id));
    auto ls_id_set_iterator = ls_id_set_.emplace(ls_id).first;

    for (auto label = label_set.begin(); label != label_set.end(); ++label) {
      if (!is_valid_label((*label).second)) [[unlikely]] {
        continue;
      }

      reverse_index_.add(label, ls_id);
      trie_index_.insert((*label).first, label.name_id(), (*label).second, label.value_id());
    }

    sorting_index_.update(ls_id_set_iterator);
  }

  PROMPP_ALWAYS_INLINE void mark_series_as_added(uint32_t ls_id) noexcept {
    if (added_series_.size() <= ls_id) [[unlikely]] {
      added_series_.resize(ls_id + 1);
    }

    added_series_.set(ls_id);
  }

  PROMPP_ALWAYS_INLINE static bool is_valid_label(std::string_view value) noexcept { return !value.empty(); }

  PROMPP_ALWAYS_INLINE static size_t phmap_hash(size_t hash) noexcept { return phmap::phmap_mix<sizeof(size_t)>()(hash); }
};

template <class QueryableEncodingBimap>
class QueryableEncodingBimapCopier {
 public:
  static constexpr auto kInvalidIdFillByte = static_cast<uint8_t>(QueryableEncodingBimap::kInvalidId);

  template <class IdsList>
  PROMPP_ALWAYS_INLINE static void resize_and_fill_ids_list(IdsList& list, uint32_t size) {
    list.resize(size);
    std::memset(list.data(), kInvalidIdFillByte, size * sizeof(typename IdsList::value_type));
  }

  template <class ItemType>
  struct Cache {
    using Item = ItemType;
    using ItemList = BareBones::Vector<ItemType>;

    template <class SymbolsTables>
    void reserve(uint32_t name_sets_count, uint32_t names_count, const SymbolsTables& symbols_tables) {
      resize_and_fill_ids_list(name_sets, name_sets_count);
      resize_and_fill_ids_list(names, names_count);

      values.resize(names_count);

      for (auto [value_cache, symbol_table] : std::ranges::views::zip(values, symbols_tables)) {
        resize_and_fill_ids_list(value_cache, symbol_table->size());
      }
    }

    ItemList name_sets;
    ItemList names;
    BareBones::Vector<ItemList> values;
  };

  QueryableEncodingBimapCopier(const QueryableEncodingBimap& source, QueryableEncodingBimap& destination) : source_(source), destination_(destination) {
    assert(destination.size() == 0);
  }

  void copy_added_series() {
    resize_and_fill_ids_list(ids_map_, source_.items_.size());

    Cache<uint32_t> cache;
    cache.reserve(source_.data_.label_name_sets_table.size(), source_.data_.label_name_sets_table.data().symbols_table.size(), source_.data_.symbols_tables);

    destination_.reserve(source_);

    for (const auto ls_id : source_.added_series_) {
      ids_map_[ls_id] = destination_.next_item_index();
      destination_.items_.emplace_back(destination_.data_, source_[ls_id], cache);
    }
  }

  void copy_ls_id_set() {
    destination_.ls_id_comparator_enabled_ = false;

    for (auto ls_id : source_.ls_id_set_) {
      if (const auto new_ls_id = ids_map_[ls_id]; new_ls_id != QueryableEncodingBimap::kInvalidId) {
        destination_.ls_id_set_.emplace_hint(destination_.ls_id_set_.end(), new_ls_id);
      }
    }

    destination_.ls_id_comparator_enabled_ = true;

    ids_map_ = BareBones::Vector<uint32_t>{};
  }

  void build_reverse_index() {
    const auto size = destination_.size();
    destination_.reverse_index_.reserve(destination_.data_.label_name_sets_table.data().symbols_table.size());

    for (uint32_t ls_id = 0; ls_id < size; ++ls_id) {
      auto label_set = destination_[ls_id];
      for (auto label = label_set.begin(); label != label_set.end(); ++label) {
        destination_.reverse_index_.add(label, ls_id);
      }
    }
  }

  void build_ls_id_hashset() {
    const auto size = destination_.size();
    destination_.ls_id_hash_set_.reserve(size);

    const auto hasher = destination_.hasher();
    for (uint32_t ls_id = 0; ls_id < size; ++ls_id) {
      destination_.ls_id_hash_set_.emplace_with_hash(QueryableEncodingBimap::phmap_hash(hasher(destination_[ls_id])),
                                                     typename QueryableEncodingBimap::Proxy(ls_id));
    }
  }

  void build_trie_index() {
    const auto& names = destination_.data_.label_name_sets_table.data().symbols_table;
    destination_.trie_index_.reserve(names.size());

    for (uint32_t name_id = 0; name_id < names.size(); ++name_id) {
      destination_.trie_index_.insert_name(names[name_id], name_id);
      destination_.trie_index_.insert_values(name_id, *destination_.data_.symbols_tables[name_id]);
    }
  }

  void copy_added_series_and_build_indexes() {
    copy_added_series();
    copy_ls_id_set();
    build_trie_index();
    build_ls_id_hashset();
    build_reverse_index();
  }

 private:
  const QueryableEncodingBimap& source_;
  QueryableEncodingBimap& destination_;
  BareBones::Vector<uint32_t> ids_map_;
};

}  // namespace series_index
