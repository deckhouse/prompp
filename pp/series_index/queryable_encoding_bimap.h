#pragma once

#include <cassert>
#include <functional>
#include <limits>
#include <ranges>
#include <span>
#include <utility>

#include "bare_bones/allocator.h"
#include "bare_bones/bitset.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/snug_composite.h"
#include "bare_bones/vector.h"
#include "parallel_hashmap/phmap.h"
#include "primitives/snug_composites.h"
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
  using SnapshotForEachSymbolIdFn = std::function<void(const std::function<void(uint32_t name_id, uint32_t value_id)>&)>;

  struct PostShrinkSnapshotAccess {
    PostShrinkResolveFn composite_resolve;
    SnapshotSymbolResolveFn symbol_resolve;
    SnapshotForEachSymbolIdFn for_each_symbol_id;
  };

  static constexpr uint32_t kPendingShrinkBoundaryNotSet = std::numeric_limits<uint32_t>::max();
  static constexpr uint32_t kKeyOnlyValueId = std::numeric_limits<uint32_t>::max();
  static constexpr uint32_t kSymbolSourceCurrent = 0;
  static constexpr uint32_t kSymbolSourceSnapshot = 1;

  friend class BareBones::SnugComposite::
      GenericDecodingTable<QueryableEncodingBimap<Vector>, PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, Vector>;

  [[nodiscard]] PROMPP_ALWAYS_INLINE const TrieIndex& trie_index() const noexcept { return trie_index_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const SeriesReverseIndex& reverse_index() const noexcept { return reverse_index_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const LsIdSet& ls_id_set() const noexcept { return ls_id_set_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const typename SortingIndexBuilder::Index& sorting_index() const noexcept { return sorting_index_.index(); }

  PROMPP_ALWAYS_INLINE void build_deferred_indexes() noexcept { sorting_index_.build(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    return trie_index_.allocated_memory() + reverse_index_.allocated_memory() + ls_id_set_allocated_memory_ + ls_id_hash_set_allocated_memory_ +
           sorting_index_.allocated_memory() + post_shrink_mapping_.allocated_memory() + Base::allocated_memory();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const auto& added_series() const noexcept { return added_series_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE typename Base::value_type resolve_impl(uint32_t id) const noexcept {
    assert(id < max_item_index_impl());
    if (is_normal()) [[likely]] {
      return Base::storage_composite(id);
    }
    if (is_fixed()) [[unlikely]] {
      return is_hidden_in_fixed_state(id) ? empty_composite() : Base::storage_composite(id);
    }
    assert(is_shrunk());
    return resolve_shrunk_series(id);
  }

  void set_pending_shrink_boundary(uint32_t boundary) noexcept {
    assert(boundary <= max_item_index_impl());
    pending_shrink_boundary_ = boundary;
    prune_hidden_series_from_hashset_in_fixed_state();
  }

  template <class Snapshot>
  void finalize_copy_and_shrink(const Snapshot& snapshot, const BareBones::Vector<uint32_t>& new_to_old_mapping) {
    finalize_copy_and_shrink(make_post_shrink_snapshot_access(snapshot), new_to_old_mapping);
  }

  template <class Callback>
  void for_each_snapshot_symbol_id(Callback&& callback) const {
    if (!is_shrunk() || !post_shrink_snapshot_access_.for_each_symbol_id) {
      return;
    }
    post_shrink_snapshot_access_.for_each_symbol_id(std::forward<Callback>(callback));
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_shrunk_for_export() const noexcept { return is_shrunk(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_fixed_for_export() const noexcept { return is_fixed(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t shift_for_export() const noexcept { return shift_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t pending_shrink_boundary_for_export() const noexcept { return pending_shrink_boundary_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE std::span<const uint32_t> post_shrink_mapping_for_export() const noexcept {
    return std::span<const uint32_t>(post_shrink_mapping_.data(), post_shrink_mapping_.size());
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const PostShrinkSnapshotAccess& post_shrink_snapshot_access_for_export() const noexcept {
    return post_shrink_snapshot_access_;
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t symbol_source_for_series(uint32_t ls_id) const noexcept { return symbol_source_for_series_impl(ls_id); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE std::string_view resolve_symbol_by_source(uint32_t source, uint32_t name_id, uint32_t value_id) const noexcept {
    if (name_id == kKeyOnlyValueId && value_id == kKeyOnlyValueId) {
      return {};
    }
    if (source == kSymbolSourceCurrent) [[likely]] {
      const auto view = this->data_view();
      return value_id == kKeyOnlyValueId ? view.key_symbol(name_id) : view.value_symbol(name_id, value_id);
    }
    if (post_shrink_snapshot_access_.symbol_resolve) {
      return post_shrink_snapshot_access_.symbol_resolve(name_id, value_id);
    }
    return {};
  }

  void shrink_to_boundary(uint32_t shrink_boundary) {
    if (shrink_boundary > next_item_index_impl()) {
      throw BareBones::Exception(0x1bf0dbff9fe3d955, "Shrink boundary [%u] exceeds table next_item_index [%u]", shrink_boundary, next_item_index_impl());
    }
    const uint32_t drop_count = shrink_boundary;

    shift_ += drop_count;
    Base::storage_.drop_front(drop_count);
    pending_shrink_boundary_ = kPendingShrinkBoundaryNotSet;

    rebuild_indexes_after_shrink();
  }

  template <class LabelSet>
  PROMPP_ALWAYS_INLINE uint32_t find_or_emplace(const LabelSet& label_set) noexcept {
    return find_or_emplace(label_set, Base::hasher()(label_set));
  }

  template <class LabelSet>
  PROMPP_ALWAYS_INLINE uint32_t find_or_emplace(const LabelSet& label_set, size_t hash) noexcept {
    hash = phmap_hash(hash);
    if (const auto existing_it = ls_id_hash_set_.find(label_set, hash); existing_it != ls_id_hash_set_.end()) {
      const auto logical_id = static_cast<uint32_t>(*existing_it);
      mark_series_as_added(logical_id);
      return logical_id;
    }

    if (is_fixed()) [[unlikely]] {
      const auto logical_id = emplace_visible_in_fixed_state(label_set, hash);
      mark_series_as_added(logical_id);
      return logical_id;
    }

    const auto logical_id = emplace_series_and_update_indexes(label_set, hash);
    mark_series_as_added(logical_id);
    return logical_id;
  }

  template <class Class>
  PROMPP_ALWAYS_INLINE std::optional<uint32_t> find(const Class& c) const noexcept {
    return find(c, Base::hasher()(c));
  }

  template <class Class>
  PROMPP_ALWAYS_INLINE std::optional<uint32_t> find(const Class& c, size_t hashval) const noexcept {
    hashval = phmap_hash(hashval);
    if (const auto i = ls_id_hash_set_.find(c, hashval); i != ls_id_hash_set_.end()) {
      return std::optional<uint32_t>{static_cast<uint32_t>(*i)};
    }
    return std::optional<uint32_t>{};
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

  void finalize_copy_and_shrink(PostShrinkSnapshotAccess snapshot_access, const BareBones::Vector<uint32_t>& new_to_old_mapping) {
    assert(snapshot_access.composite_resolve && snapshot_access.symbol_resolve);
    assert(pending_shrink_boundary_ != kPendingShrinkBoundaryNotSet);
    assert(pending_shrink_boundary_ <= next_item_index_impl());

    invert_copy_mapping(new_to_old_mapping);
    post_shrink_snapshot_access_ = std::move(snapshot_access);
    shrink_to_boundary(pending_shrink_boundary_);
  }

  void invert_copy_mapping(const BareBones::Vector<uint32_t>& new_to_old) {
    post_shrink_mapping_.clear();
    post_shrink_mapping_.resize(pending_shrink_boundary_);

    std::fill(post_shrink_mapping_.begin(), post_shrink_mapping_.end(), BareBones::SnugComposite::kInvalidLsId);

    for (size_t new_id = 0; new_id < new_to_old.size(); ++new_id) {
      const uint32_t old_id = new_to_old[new_id];
      if (old_id < post_shrink_mapping_.size()) [[likely]] {
        post_shrink_mapping_[old_id] = static_cast<uint32_t>(new_id);
      }
    }
  }

  template <class Snapshot>
  [[nodiscard]] static PostShrinkSnapshotAccess make_post_shrink_snapshot_access(const Snapshot& snapshot) {
    // snapshot object must outlive the shrunk table state that uses these callbacks.
    PostShrinkSnapshotAccess access;
    access.composite_resolve = [&snapshot](uint32_t id) { return snapshot[id]; };
    access.symbol_resolve = [&snapshot](uint32_t name_id, uint32_t value_id) { return resolve_snapshot_symbol(snapshot, name_id, value_id); };
    access.for_each_symbol_id = [&snapshot](const auto& callback) { enumerate_snapshot_symbol_ids(snapshot, callback); };
    return access;
  }

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
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t max_item_index_impl() const noexcept { return next_item_index_impl(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t series_count_impl() const noexcept {
    if (!is_shrunk()) [[likely]] {
      return Base::storage_.count();
    }

    uint32_t mapped_visible = 0;
    const uint32_t mapping_boundary = std::min<uint32_t>(shift_, post_shrink_mapping_.size());
    for (uint32_t logical_id = 0; logical_id < mapping_boundary; ++logical_id) {
      if (post_shrink_mapping_[logical_id] != Base::kInvalidId) [[likely]] {
        ++mapped_visible;
      }
    }
    return Base::storage_.count() + mapped_visible;
  }

  [[nodiscard]] static PROMPP_ALWAYS_INLINE typename Base::value_type empty_composite() noexcept { return typename Base::value_type{}; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_normal() const noexcept { return shift_ == 0 && pending_shrink_boundary_ == kPendingShrinkBoundaryNotSet; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_shrunk() const noexcept { return shift_ > 0; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_fixed() const noexcept { return shift_ == 0 && pending_shrink_boundary_ != kPendingShrinkBoundaryNotSet; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_hidden_in_fixed_state(uint32_t ls_id) const noexcept {
    assert(is_fixed());
    assert(ls_id < added_series_.size());
    return ls_id < pending_shrink_boundary_ && !is_series_alive(ls_id);
  }

  template <class LabelSetLike>
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t emplace_series_and_update_indexes(const LabelSetLike& label_set, size_t mixed_hash) {
    const auto new_storage_id = Base::storage_.emplace_back(label_set);
    const auto composite_label_set = Base::storage_composite(new_storage_id);
    const auto new_logical_id = shift_ + new_storage_id;

    ls_id_hash_set_.emplace_with_hash(mixed_hash, typename Base::Proxy(new_logical_id));
    update_indexes(new_logical_id, composite_label_set);
    return new_logical_id;
  }

  template <class LabelSetLike>
  [[nodiscard]] PROMPP_ATTRIBUTE_NOINLINE uint32_t emplace_visible_in_fixed_state(const LabelSetLike& label_set, size_t mixed_hash) {
    assert(is_fixed());
    const auto new_storage_id = Base::storage_.emplace_back(label_set);
    const auto logical_id = shift_ + new_storage_id;
    assert(logical_id >= pending_shrink_boundary_);

    mark_series_as_added(logical_id);
    const auto composite_label_set = Base::storage_composite(new_storage_id);
    ls_id_hash_set_.emplace_with_hash(mixed_hash, typename Base::Proxy(logical_id));
    update_indexes(logical_id, composite_label_set);
    return logical_id;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE typename Base::value_type resolve_shrunk_series(uint32_t ls_id) const noexcept {
    assert(is_shrunk());
    if (ls_id >= shift_) [[likely]] {
      return Base::storage_composite(ls_id - shift_);
    }
    assert(!post_shrink_mapping_.empty() && post_shrink_snapshot_access_.composite_resolve);
    const auto mapped_id = post_shrink_mapping_[ls_id];
    if (mapped_id == Base::kInvalidId) [[unlikely]] {
      return empty_composite();
    }
    return post_shrink_snapshot_access_.composite_resolve(mapped_id);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t symbol_source_for_series_impl(uint32_t ls_id) const noexcept {
    if (!is_shrunk()) [[likely]] {
      return kSymbolSourceCurrent;
    }
    return ls_id < shift_ ? kSymbolSourceSnapshot : kSymbolSourceCurrent;
  }

  template <class Snapshot, class Callback>
  static void enumerate_snapshot_symbol_ids(const Snapshot& snapshot, Callback&& callback) {
    const auto view = snapshot.data_view();
    for (auto it = view.values().begin(), e = view.values().end(); it != e; ++it) {
      callback(it.key_id(), it.value_id());
    }
  }

  template <class Snapshot>
  [[nodiscard]] static std::string_view resolve_snapshot_symbol(const Snapshot& snapshot, uint32_t name_id, uint32_t value_id) {
    const auto view = snapshot.data_view();
    return value_id == kKeyOnlyValueId ? view.key_symbol(name_id) : view.value_symbol(name_id, value_id);
  }

  uint32_t shift_{0};
  uint32_t pending_shrink_boundary_{kPendingShrinkBoundaryNotSet};
  BareBones::Vector<uint32_t> post_shrink_mapping_{};
  PostShrinkSnapshotAccess post_shrink_snapshot_access_{};

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_series_alive(uint32_t logical_id) const noexcept {
    assert(logical_id < added_series_.size());
    return added_series_[logical_id];
  }

  void prune_hidden_series_from_hashset_in_fixed_state() noexcept {
    assert(is_fixed());
    const uint32_t boundary = pending_shrink_boundary_;
    assert(boundary <= added_series_.size());
    for (auto zero_it = added_series_.zbegin(); zero_it != added_series_.zend(); ++zero_it) {
      if (*zero_it >= boundary) {
        break;
      }
      ls_id_hash_set_.erase(typename Base::Proxy(*zero_it));
    }
  }

  void rebuild_indexes_after_shrink() {
    assert(is_shrunk());
    for (auto it = ls_id_set_.begin(); it != ls_id_set_.end();) {
      const auto logical_id = static_cast<uint32_t>(*it);
      if (logical_id >= shift_) [[likely]] {
        ++it;
        continue;
      }
      if (!is_series_alive(logical_id)) [[unlikely]] {
        it = ls_id_set_.erase(it);
        continue;
      }
      ++it;
    }
    sorting_index_.clear();
    sorting_index_.build();
  }
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
    assert(destination.series_count() == 0);
  }

  void copy_added_series() {
    old_new_ids_.clear();
    old_new_ids_.reserve(source_.series_count());

    Cache<uint32_t> cache;
    cache.reserve(source_.data_view());

    destination_.reserve(source_);

    dst_src_ids_mapping_.clear();
    dst_src_ids_mapping_.reserve(source_.series_count());

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
    const auto size = destination_.series_count();
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
    const auto size = destination_.series_count();
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

}  // namespace series_index
