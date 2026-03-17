#pragma once

#include <cassert>
#include <functional>
#include <limits>
#include <ranges>

#include "bare_bones/allocator.h"
#include "bare_bones/bitset.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/snug_composite.h"
#include "primitives/snug_composites.h"
#include "queried_series.h"
#include "reverse_index.h"
#include "series_index/trie/cedarpp_tree.h"
#include "sorting_index.h"
#include "trie_index.h"

namespace series_index {

template <template <class> class Vector>
class QueryableEncodingBimap final : public BareBones::SnugComposite::GenericDecodingTable<QueryableEncodingBimap<Vector>,
                                                                                           PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament,
                                                                                           Vector> {
 public:
  using Base = BareBones::SnugComposite::
      GenericDecodingTable<QueryableEncodingBimap<Vector>, PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, Vector>;
  using LsIdSet = phmap::btree_set<typename Base::Proxy, typename Base::LessComparator, BareBones::Allocator<typename Base::Proxy>>;
  using HashSet =
      phmap::flat_hash_set<typename Base::Proxy, typename Base::Hasher, typename Base::EqualityComparator, BareBones::Allocator<typename Base::Proxy>>;
  using LsIdSetIterator = typename LsIdSet::const_iterator;
  using SortingIndexBuilder = series_index::SortingIndexBuilder<LsIdSet, Vector>;
  using TrieIndex = series_index::TrieIndex<trie::CedarTrie>;
  using TrieIndexIterator = typename TrieIndex::Iterator;
  using checkpoint_type = typename Base::checkpoint_type;
  using PostShrinkResolveFn = std::function<typename Base::value_type(uint32_t)>;
  using SnapshotSymbolResolveFn = std::function<std::string_view(uint32_t name_id, uint32_t value_id)>;

  static constexpr uint32_t kPendingShrinkBoundaryNotSet = std::numeric_limits<uint32_t>::max();
  static constexpr uint32_t kKeyOnlyValueId = std::numeric_limits<uint32_t>::max();

  friend class BareBones::SnugComposite::
      GenericDecodingTable<QueryableEncodingBimap<Vector>, PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, Vector>;

 public:
  [[nodiscard]] PROMPP_ALWAYS_INLINE const TrieIndex& trie_index() const noexcept { return trie_index_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const SeriesReverseIndex& reverse_index() const noexcept { return reverse_index_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const LsIdSet& ls_id_set() const noexcept { return ls_id_set_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const typename SortingIndexBuilder::Index& sorting_index() const noexcept { return sorting_index_.index(); }

  // TODO: review and remove unnecessary calls of this method in code
  PROMPP_ALWAYS_INLINE void build_deferred_indexes() noexcept { sorting_index_.build(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    return trie_index_.allocated_memory() + reverse_index_.allocated_memory() + ls_id_set_allocated_memory_ + ls_id_hash_set_allocated_memory_ +
           sorting_index_.allocated_memory() + Base::allocated_memory();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const auto& added_series() const noexcept { return added_series_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE typename Base::value_type operator[](uint32_t id) const noexcept {
    if (shift_ == 0) [[likely]] {
      if (pending_shrink_boundary_ != kPendingShrinkBoundaryNotSet && id < pending_shrink_boundary_) [[unlikely]] {
        if (id >= added_series_.size() || !added_series_[id]) {
          return empty_composite();
        }
      }
      return Base::operator[](id);
    }
    if (id >= shift_) [[likely]] {
      return Base::operator[](id - shift_);
    }
    assert(post_shrink_mapping_ptr_ != nullptr && post_shrink_resolve_);
    const auto new_id = (*post_shrink_mapping_ptr_)[id];
    if (new_id == Base::kInvalidId) [[unlikely]] {
      return empty_composite();
    }
    return post_shrink_resolve_(new_id);
  }

  void set_pending_shrink_boundary(uint32_t boundary) noexcept { pending_shrink_boundary_ = boundary; }

  static constexpr uint32_t kSymbolSourceCurrent = 0;
  static constexpr uint32_t kSymbolSourceSnapshot = 1;

  [[nodiscard]] uint32_t symbol_source_for_series(uint32_t ls_id) const noexcept {
    if (shift_ == 0) [[likely]] {
      return kSymbolSourceCurrent;
    }
    return ls_id < shift_ ? kSymbolSourceSnapshot : kSymbolSourceCurrent;
  }

  template <class Callback>
  void enumerate_symbol_ids(Callback&& callback) const {
    if (shift_ == 0) [[likely]] {
      const auto view = this->data_view();
      for (auto it = view.keys().begin(), e = view.keys().end(); it != e; ++it) {
        callback(kSymbolSourceCurrent, it.id(), kKeyOnlyValueId);
      }
      for (auto it = view.values().begin(), e = view.values().end(); it != e; ++it) {
        callback(kSymbolSourceCurrent, it.key_id(), it.value_id());
      }
      return;
    }
    for (uint32_t id = 0; id < next_item_index_impl(); ++id) {
      const uint32_t src = id >= shift_ ? kSymbolSourceCurrent : kSymbolSourceSnapshot;
      const auto composite = (*this)[id];
      for (auto label = composite.begin(); label != composite.end(); ++label) {
        callback(src, label.name_id(), kKeyOnlyValueId);
        callback(src, label.name_id(), label.value_id());
      }
    }
  }

  [[nodiscard]] std::string_view resolve_symbol(uint32_t source, uint32_t name_id, uint32_t value_id) const noexcept {
    if (name_id == kKeyOnlyValueId && value_id == kKeyOnlyValueId) {
      return {};
    }
    if (source == kSymbolSourceCurrent) [[likely]] {
      const auto view = this->data_view();
      return value_id == kKeyOnlyValueId ? view.key_symbol(name_id) : view.value_symbol(name_id, value_id);
    }
    if (post_shrink_snapshot_resolve_symbol_) {
      return post_shrink_snapshot_resolve_symbol_(name_id, value_id);
    }
    return {};
  }

  void fill_touched_series_mapping([[maybe_unused]] uint32_t shrink_boundary,
                                   QueryableEncodingBimap& copy,
                                   BareBones::Vector<uint32_t>& old_to_new_mapping,
                                   const BareBones::Bitset& touched_series) {
    const uint32_t max_lsid = next_item_index_impl();
    assert(shrink_boundary <= max_lsid && old_to_new_mapping.size() >= max_lsid);

    for (uint32_t old_id = 0; old_id < max_lsid; ++old_id) {
      if (old_id < touched_series.size() && touched_series[old_id] && old_to_new_mapping[old_id] == Base::kInvalidId) [[unlikely]] {
        const auto label_set = (*this)[old_id];
        const auto new_id = copy.find_or_emplace(label_set);
        old_to_new_mapping[old_id] = new_id;
      }
    }
  }

  template <class SnapshotCompositeResolveFn, class SnapshotSymbolResolveFn>
  void finalize_copy_and_shrink(uint32_t shrink_boundary,
                                SnapshotCompositeResolveFn&& snapshot_composite_resolve,
                                SnapshotSymbolResolveFn&& snapshot_symbol_resolve,
                                BareBones::Vector<uint32_t>& old_to_new_mapping) {
    assert(shrink_boundary <= next_item_index_impl() && old_to_new_mapping.size() >= next_item_index_impl());

    shrink_to_boundary(shrink_boundary);
    post_shrink_mapping_ptr_ = &old_to_new_mapping;
    post_shrink_resolve_ = std::forward<SnapshotCompositeResolveFn>(snapshot_composite_resolve);
    post_shrink_snapshot_resolve_symbol_ = SnapshotSymbolResolveFn(std::forward<SnapshotSymbolResolveFn>(snapshot_symbol_resolve));
  }

  void finalize_copy_and_shrink(uint32_t shrink_boundary, QueryableEncodingBimap& copy, BareBones::Vector<uint32_t>& old_to_new_mapping) {
    finalize_copy_and_shrink(
        shrink_boundary, [&copy](uint32_t id) { return copy[id]; },
        [&copy](uint32_t name_id, uint32_t value_id) {
          const auto& view = copy.data_view();
          return value_id == kKeyOnlyValueId ? view.key_symbol(name_id) : view.value_symbol(name_id, value_id);
        },
        old_to_new_mapping);
  }

  void shrink_to_boundary(uint32_t shrink_boundary) {
    if (shrink_boundary > next_item_index_impl()) {
      throw BareBones::Exception(0x1bf0dbff9fe3d955, "Shrink boundary [%u] exceeds table next_item_index [%u]", shrink_boundary, next_item_index_impl());
    }
    const uint32_t drop_count = shrink_boundary;

    shift_ += drop_count;
    Base::storage_.drop_front(drop_count);

    added_series_.clear();
    pending_shrink_boundary_ = kPendingShrinkBoundaryNotSet;
  }

  template <class LabelSet>
  PROMPP_ALWAYS_INLINE uint32_t find_or_emplace(const LabelSet& label_set) noexcept {
    return find_or_emplace(label_set, Base::hasher()(label_set));
  }

  template <class LabelSet>
  PROMPP_ALWAYS_INLINE uint32_t find_or_emplace(const LabelSet& label_set, size_t hash) noexcept {
    hash = phmap_hash(hash);
    const auto storage_id = *ls_id_hash_set_.lazy_emplace_with_hash(label_set, hash, [&](const auto& ctor) {
      auto new_storage_id = Base::storage_.emplace_back(label_set);
      const auto composite_label_set = Base::operator[](new_storage_id);
      ctor(typename Base::Proxy(new_storage_id));
      update_indexes(shift_ + new_storage_id, composite_label_set);
      return new_storage_id;
    });

    const auto logical_id = shift_ + storage_id;
    if (pending_shrink_boundary_ == kPendingShrinkBoundaryNotSet) {
      mark_series_as_added(logical_id);
    }
    return logical_id;
  }

  template <class Class>
  PROMPP_ALWAYS_INLINE std::optional<uint32_t> find(const Class& c) const noexcept {
    return find(c, Base::hasher()(c));
  }

  template <class Class>
  PROMPP_ALWAYS_INLINE std::optional<uint32_t> find(const Class& c, size_t hashval) const noexcept {
    auto i = ls_id_hash_set_.find(c, phmap_hash(hashval));
    if (i == ls_id_hash_set_.end()) [[unlikely]] {
      return {};
    }
    const auto logical_id = shift_ + static_cast<uint32_t>(*i);
    if (shift_ == 0) [[likely]] {
      if (pending_shrink_boundary_ != kPendingShrinkBoundaryNotSet && logical_id < pending_shrink_boundary_) [[unlikely]] {
        if (logical_id >= added_series_.size() || !added_series_[logical_id]) {
          return {};
        }
      }
      return logical_id;
    }
    if (logical_id >= shift_) [[likely]] {
      return logical_id;
    }
    if (post_shrink_mapping_ptr_ != nullptr && (*post_shrink_mapping_ptr_)[logical_id] != Base::kInvalidId) {
      return logical_id;
    }
    return {};
  }

  using Base::reserve;
  PROMPP_ALWAYS_INLINE void reserve(uint32_t count) {
    Base::reserve(count);
    ls_id_hash_set_.reserve(count);
    added_series_.reserve(count);
  }

 private:
  using LabelSet = typename Base::value_type;
  using Trie = trie::CedarTrie;

  template <class DecodingTable, class SortingIndex, class SeriesIds, class QueryableEncodingBimap, class LsIdVector>
  friend class QueryableEncodingBimapCopier;

  [[nodiscard]] auto& storage() noexcept { return this->storage_; }

  TrieIndex trie_index_;
  SeriesReverseIndex reverse_index_;

  size_t ls_id_set_allocated_memory_{};
  LsIdSet ls_id_set_{{}, Base::less_comparator(), BareBones::Allocator<typename Base::Proxy>{ls_id_set_allocated_memory_}};

  size_t ls_id_hash_set_allocated_memory_{};
  HashSet ls_id_hash_set_{0, Base::hasher(), Base::equality_comparator(), BareBones::Allocator<typename Base::Proxy>{ls_id_hash_set_allocated_memory_}};

  SortingIndexBuilder sorting_index_{ls_id_set_};

  BareBones::Bitset added_series_;

  template <BareBones::SnugComposite::ls_id_range R>
  PROMPP_ALWAYS_INLINE void after_items_load_impl(R&& loaded_ids) noexcept {
    if constexpr (std::ranges::sized_range<R>) {
      ls_id_hash_set_.reserve(std::ranges::size(loaded_ids));
    }

    const auto hasher = Base::hasher();
    for (const auto ls_id : loaded_ids) {
      auto label_set = this->operator[](ls_id);
      ls_id_hash_set_.emplace_with_hash(phmap_hash(hasher(label_set)), typename Base::Proxy(ls_id));
      update_indexes(ls_id, label_set);
    }
  }

  void update_indexes(uint32_t ls_id, const LabelSet& label_set) {
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

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t next_item_index_impl() const noexcept { return shift_ + Base::storage_.count(); }

  [[nodiscard]] static PROMPP_ALWAYS_INLINE typename Base::value_type empty_composite() noexcept { return typename Base::value_type{}; }

  uint32_t shift_{0};
  uint32_t pending_shrink_boundary_{kPendingShrinkBoundaryNotSet};
  const BareBones::Vector<uint32_t>* post_shrink_mapping_ptr_{nullptr};
  PostShrinkResolveFn post_shrink_resolve_;
  SnapshotSymbolResolveFn post_shrink_snapshot_resolve_symbol_;
};

template <class DecodingTable, class SortingIndex, class SeriesIds, class QueryableEncodingBimap, class LsIdVector>
class QueryableEncodingBimapCopier {
 public:
  static constexpr auto kInvalidIdFillByte = static_cast<uint8_t>(DecodingTable::kInvalidId);

  template <class IdsList>
  PROMPP_ALWAYS_INLINE static void resize_and_fill_ids_list(IdsList& list, uint32_t size) {
    list.resize(size);
    std::memset(list.data(), kInvalidIdFillByte, size * sizeof(typename IdsList::value_type));
  }

  template <class ItemType>
  struct Cache {
    using Item = ItemType;
    using ItemList = BareBones::Vector<ItemType>;

    void reserve(const auto source_view) {
      const auto names_count = source_view.keys().size();

      resize_and_fill_ids_list(name_sets, source_view.label_name_sets().size());
      resize_and_fill_ids_list(names, names_count);

      values.resize(names_count);

      uint32_t v_id = 0;
      for (auto it = source_view.keys().begin(), e = source_view.keys().end(); it != e; ++it, ++v_id) {
        auto& value_cache = values[v_id];

        resize_and_fill_ids_list(value_cache, source_view.values(it.id()).size());
      }
    }

    ItemList name_sets;
    ItemList names;
    BareBones::Vector<ItemList> values;
  };

  QueryableEncodingBimapCopier(const DecodingTable& source,
                               const SortingIndex& sorting_index,
                               const SeriesIds& ls_id_range,
                               QueryableEncodingBimap& destination,
                               LsIdVector& dst_src_ids_mapping)
      : source_(source), sorting_index_(sorting_index), ls_id_range_(ls_id_range), destination_(destination), dst_src_ids_mapping_(dst_src_ids_mapping) {
    assert(destination.size() == 0);
  }

  void copy_added_series() {
    old_new_ids_.clear();
    old_new_ids_.reserve(source_.size());

    Cache<uint32_t> cache;
    cache.reserve(source_.data_view());

    destination_.reserve(source_);

    dst_src_ids_mapping_.clear();
    dst_src_ids_mapping_.reserve(source_.size());

    for (const auto ls_id : ls_id_range_) {
      old_new_ids_.emplace_back(ls_id, destination_.next_item_index());
      dst_src_ids_mapping_.emplace_back(ls_id);
      destination_.storage().emplace_back(source_[ls_id], cache);
    }

    const auto cmp = sorting_index_.get_comparator();
    std::sort(old_new_ids_.begin(), old_new_ids_.end(), [&](const id_pair& a, const id_pair& b) { return cmp(a.old_id, b.old_id); });
  }

  void copy_ls_id_set() {
    for (const auto& p : old_new_ids_) {
      destination_.ls_id_set_.emplace_hint_cmp(destination_.ls_id_set_.end(), [](auto, auto) { return true; }, p.new_id);
    }

    old_new_ids_ = {};
  }

  void build_reverse_index() {
    const auto size = destination_.size();
    const auto names_view = destination_.data_view().keys();
    destination_.reverse_index_.reserve(names_view.size());

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
    const auto view = destination_.data_view();
    const auto names_view = destination_.data_view().keys();
    destination_.trie_index_.reserve(names_view.size());

    for (auto name_it = names_view.begin(); name_it != names_view.end(); ++name_it) {
      const uint32_t name_id = name_it.id();
      destination_.trie_index_.insert_name(*name_it, name_id);
      destination_.trie_index_.insert_values(name_id, view.values(name_id));
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
  struct id_pair {
    uint32_t old_id;
    uint32_t new_id;
  };

  BareBones::Vector<id_pair> old_new_ids_;
  const DecodingTable& source_;
  const SortingIndex& sorting_index_;
  const SeriesIds& ls_id_range_;
  QueryableEncodingBimap& destination_;
  LsIdVector& dst_src_ids_mapping_;
};

template <class NewToOldContainer>
inline void invert_copy_mapping(const NewToOldContainer& new_to_old, uint32_t max_lsid, BareBones::Vector<uint32_t>& old_to_new_out) {
  old_to_new_out.clear();
  old_to_new_out.resize(max_lsid);

  std::fill(old_to_new_out.begin(), old_to_new_out.end(), BareBones::SnugComposite::kInvalidLsId);

  for (size_t new_id = 0; new_id < new_to_old.size(); ++new_id) {
    const uint32_t old_id = static_cast<uint32_t>(new_to_old[new_id]);
    if (old_id < max_lsid) [[likely]] {
      old_to_new_out[old_id] = static_cast<uint32_t>(new_id);
    }
  }
}

}  // namespace series_index
