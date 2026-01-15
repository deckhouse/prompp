#pragma once

#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Warray-bounds"
#include <parallel_hashmap/btree.h>
#include <parallel_hashmap/phmap.h>
#pragma GCC diagnostic pop

#include <scope_exit.h>

#include <ranges>

#include "bare_bones/allocator.h"
#include "bare_bones/exception.h"
#include "bare_bones/streams.h"
#include "bare_bones/vector.h"
#include "bare_bones/xxhash.h"

namespace BareBones::SnugComposite {

/**
 * Serialization mode used to annotate encoded data and how to apply it on container.
 * We use Delta to save the difference between two states of container. It doesn't matter
 * how the first state was made (from snapshot or other delta). Snapshot may be explained
 * as a delta from the init (zero) state.
 */
enum class SerializationMode : char { SNAPSHOT = 1, DELTA = 2 };

template <class Range>
concept ls_id_range = std::ranges::range<Range> && std::same_as<std::ranges::range_value_t<Range>, uint32_t>;

template <class FilamentStorageType>
concept is_shrinkable = requires(FilamentStorageType& storage) {
  { storage.shrink() };
};

template <class Derived>
concept has_next_item_index = requires(Derived derived) {
  { derived.next_item_index_impl() };
};

template <class Derived, class Checkpoint>
concept has_rollback = requires(Derived derived, const Checkpoint& checkpoint) {
  { derived.rollback_impl(checkpoint) };
};

template <class Derived, class R>
concept has_after_items_load = ls_id_range<R> && requires(Derived derived, R&& range) { derived.after_items_load_impl(std::forward<R>(range)); };

template <class Derived, template <template <class> class> class Filament, template <class> class Vector>
class GenericDecodingTable {
  static_assert(!std::is_integral_v<typename Filament<Vector>::storage_type::composite_type>, "Filament::composite_type can't be an integral type");

  template <class AnyDerived, template <template <class> class> class AnyFilament, template <class> class AnyVector>
  friend class GenericDecodingTable;

 public:
  using filament_type = Filament<Vector>;
  using storage_type = filament_type::storage_type;
  using value_type = storage_type::composite_type;

  static constexpr bool kIsReadOnly = IsSharedSpan<Vector<uint8_t>>::value;

  static constexpr auto kInvalidId = std::numeric_limits<uint32_t>::max();

 protected:
  class Proxy {
    uint32_t id_;

   public:
    // NOLINTNEXTLINE(google-explicit-constructor)
    PROMPP_ALWAYS_INLINE Proxy(uint32_t id) noexcept : id_(id) {}

    // NOLINTNEXTLINE(google-explicit-constructor)
    PROMPP_ALWAYS_INLINE operator uint32_t() const noexcept { return id_; }

    PROMPP_ALWAYS_INLINE bool operator==(const Proxy& o) noexcept { return id_ == o.id_; }
  };

  struct Hasher {
    using is_transparent = void;

    const GenericDecodingTable* decoding_table;
    PROMPP_ALWAYS_INLINE explicit Hasher(const GenericDecodingTable* _decoding_table = nullptr) noexcept : decoding_table(_decoding_table) {}

    PROMPP_ALWAYS_INLINE size_t operator()(const std::string_view& str) const noexcept { return XXHash3::hash(str); }
    PROMPP_ALWAYS_INLINE size_t operator()(const std::string& str) const noexcept { return XXHash3::hash(str); }

    template <class Class>
    PROMPP_ALWAYS_INLINE size_t operator()(const Class& c) const noexcept {
      return phmap::Hash<Class>()(c);
    }

    PROMPP_ALWAYS_INLINE size_t operator()(const Proxy& p) const noexcept { return this->operator()(decoding_table->storage_.composite(p)); }
  };

  struct EqualityComparator {
    using is_transparent = void;

    const GenericDecodingTable* decoding_table;
    PROMPP_ALWAYS_INLINE explicit EqualityComparator(const GenericDecodingTable* _decoding_table = nullptr) noexcept : decoding_table(_decoding_table) {}

    PROMPP_ALWAYS_INLINE bool operator()(const Proxy& a, const Proxy& b) const noexcept { return a == b; }

    template <class Class>
    PROMPP_ALWAYS_INLINE bool operator()(const Proxy& a, const Class& b) const noexcept {
      return decoding_table->storage_.composite(a) == b;
    }
  };

  struct LessComparator {
    using is_transparent = void;

    PROMPP_ALWAYS_INLINE explicit LessComparator(const GenericDecodingTable* decoding_table) noexcept : decoding_table_(decoding_table) {}

    PROMPP_ALWAYS_INLINE bool operator()(const Proxy& a, const Proxy& b) const noexcept {
      return decoding_table_->storage_.composite(a) < decoding_table_->storage_.composite(b);
    }

    template <class Class>
    PROMPP_ALWAYS_INLINE bool operator()(const Proxy& a, const Class& b) const noexcept {
      return decoding_table_->storage_.composite(a) < b;
    }

    template <class Class>
      requires(!std::is_same_v<Class, Proxy>)
    PROMPP_ALWAYS_INLINE bool operator()(const Class& a, const Proxy& b) const noexcept {
      return a < decoding_table_->storage_.composite(b);
    }

   private:
    const GenericDecodingTable* decoding_table_;
  };

  class Checkpoint {
    const storage_type* storage_ptr_;
    uint32_t next_item_index_;
    uint32_t size_;
    typename storage_type::checkpoint_type storage_checkpoint_;
    uint8_t table_version_;

   public:
    explicit PROMPP_ALWAYS_INLINE Checkpoint(const storage_type& storage, uint32_t next_item_index, uint32_t size, uint8_t table_version) noexcept
        : storage_ptr_(&storage), next_item_index_(next_item_index), size_(size), storage_checkpoint_(storage.checkpoint()), table_version_(table_version) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size() const noexcept { return size_; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t next_item_index() const noexcept { return next_item_index_; }

    const typename storage_type::checkpoint_type& storage_checkpoint() const noexcept { return storage_checkpoint_; }

    template <OutputStream S>
    void save(S& out, const Checkpoint* from = nullptr) const {
      auto original_exceptions = out.exceptions();
      auto sg1 = std::experimental::scope_exit([&]() { out.exceptions(original_exceptions); });
      out.exceptions(std::ifstream::failbit | std::ifstream::badbit);

      // write a version
      out.put(table_version_);

      // write mode
      SerializationMode mode = (from != nullptr) ? SerializationMode::DELTA : SerializationMode::SNAPSHOT;
      out.put(static_cast<char>(mode));

      // write index of the first item in the portion
      uint32_t first_to_save_i = 0;
      if (from != nullptr) {
        first_to_save_i = from->next_item_index_;
        out.write(reinterpret_cast<const char*>(&from->next_item_index_), sizeof(from->next_item_index_));
      }
      const uint32_t first_item_index_in_decoding_table = next_item_index_ - size_;
      const uint32_t id_offset = first_to_save_i - first_item_index_in_decoding_table;
      assert(first_to_save_i >= first_item_index_in_decoding_table);

      // write size
      uint32_t size_to_save = next_item_index_ - first_to_save_i;
      out.write(reinterpret_cast<char*>(&size_to_save), sizeof(size_to_save));
      // if there are no items to write, we finish here
      if (!size_to_save) {
        return;
      }

      // write data
      if (from != nullptr) {
        storage_checkpoint_.save(out, *storage_ptr_, id_offset, size_to_save, table_version_, &from->storage_checkpoint_);
      } else {
        storage_checkpoint_.save(out, *storage_ptr_, id_offset, size_to_save, table_version_);
      }
    }

    template <OutputStream S>
    friend S& operator<<(S& out, const Checkpoint& cp) {
      cp.save(out);
      return out;
    }

    size_t save_size(const Checkpoint* from = nullptr) const noexcept {
      // version is written and read by methods put() and get() and they write and read 1 byte
      size_t res = 1 + sizeof(SerializationMode);

      // index of the first item in the portion
      uint32_t first_to_save_i = 0;
      if (from != nullptr) {
        first_to_save_i = from->next_item_index_;
        res += sizeof(uint32_t);
      }

      // size
      const uint32_t size_to_save = next_item_index_ - first_to_save_i;
      res += sizeof(uint32_t);

      // if there are no items to write, we finish here
      if (!size_to_save)
        return res;

      // data
      if (from != nullptr) {
        res += storage_checkpoint_.save_size(*storage_ptr_, size_to_save, &from->storage_checkpoint_);
      } else {
        res += storage_checkpoint_.save_size(*storage_ptr_, size_to_save);
      }

      return res;
    }

    /**
     * ATTENTION! This class persists only pointers to checkpoint. It's a user responsibility
     * to prevent using delta out of checkpoint scope!
     */
    class Delta {
      Checkpoint const* from_;
      Checkpoint const* to_;

     public:
      PROMPP_ALWAYS_INLINE Delta(Checkpoint const& from, Checkpoint const& to) noexcept : from_(&from), to_(&to) {}

      [[nodiscard]] bool empty() const noexcept { return from_->size() >= to_->size(); }

      template <OutputStream S>
      friend S& operator<<(S& out, Delta dt) {
        dt.to_->save(out, dt.from_);
        return out;
      }

      [[nodiscard]] size_t save_size() const noexcept { return to_->save_size(from_); }
    };

    Delta operator-(const Checkpoint& from) const noexcept { return Delta(from, *this); }
  };

  [[nodiscard]] PROMPP_ALWAYS_INLINE Hasher hasher() const noexcept { return Hasher(this); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE EqualityComparator equality_comparator() const noexcept { return EqualityComparator(this); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE LessComparator less_comparator() const noexcept { return LessComparator(this); }

  storage_type storage_;
  uint8_t version_{1};

 public:
  using checkpoint_type = Checkpoint;
  using delta_type = typename checkpoint_type::Delta;

  GenericDecodingTable() noexcept = default;
  explicit GenericDecodingTable(uint8_t version) noexcept : version_(version){};

  template <class AnotherDerived, template <template <class> class> class AnotherFilament, template <class> class AnotherVector>
    requires kIsReadOnly
  explicit GenericDecodingTable(const GenericDecodingTable<AnotherDerived, AnotherFilament, AnotherVector>& other)
      : storage_(other.storage_), version_(other.version_) {}

  GenericDecodingTable(const GenericDecodingTable& other) = delete;
  GenericDecodingTable& operator=(const GenericDecodingTable& other) = delete;

  GenericDecodingTable(GenericDecodingTable&& other) noexcept = delete;
  GenericDecodingTable& operator=(GenericDecodingTable&& other) noexcept = delete;

  PROMPP_ALWAYS_INLINE value_type operator[](uint32_t id) const noexcept { return storage_.composite(id); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return storage_.count(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return mem::allocated_memory(storage_); }

  PROMPP_ALWAYS_INLINE storage_type::view_type data_view() const noexcept { return storage_.view(); }

  template <class DerivedOther, template <template <class> class> class FilamentOther, template <class> class VectorOther>
  PROMPP_ALWAYS_INLINE void reserve(const GenericDecodingTable<DerivedOther, FilamentOther, VectorOther>& other) {
    storage_.reserve(other.storage_);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t next_item_index() const noexcept {
    if constexpr (has_next_item_index<Derived>) {
      return static_cast<const Derived*>(this)->next_item_index_impl();
    } else {
      return storage_.next_item_index();
    }
  }

  PROMPP_ALWAYS_INLINE auto checkpoint() const noexcept { return checkpoint_type(storage_, next_item_index(), size(), version_); }

  PROMPP_ALWAYS_INLINE void rollback(const checkpoint_type& checkpoint) noexcept
    requires(!kIsReadOnly)
  {
    if constexpr (has_rollback<Derived, checkpoint_type>) {
      static_cast<Derived*>(this)->rollback_impl(checkpoint);
    }

    if constexpr (!kIsReadOnly) {
      assert(checkpoint.size() <= size());
      storage_.rollback(checkpoint.storage_checkpoint());
    }
  }

  template <InputStream S>
  void load(S& in) {
    // read version
    const uint8_t version = in.get();

    // return successfully if the stream is empty
    if (in.eof()) {
      return;
    }

    // check version
    if (version != 1 && version != 2) {
      throw BareBones::Exception(0x343b7bcc6814f2d5, "Invalid EncodingSequence version %d got from input stream, only versions 1 and 2 are supported", version);
    }

    auto original_exceptions = in.exceptions();
    auto sg1 = std::experimental::scope_exit([&]() { in.exceptions(original_exceptions); });
    in.exceptions(std::ifstream::failbit | std::ifstream::badbit | std::ifstream::eofbit);

    // read mode
    const auto mode = static_cast<SerializationMode>(in.get());

    // read index of the first item in the portion if we are reading wal
    uint32_t first_to_load_i = 0;
    if (mode == SerializationMode::DELTA) {
      in.read(reinterpret_cast<char*>(&first_to_load_i), sizeof(first_to_load_i));
    }
    if (first_to_load_i != next_item_index()) {
      if (mode == SerializationMode::SNAPSHOT) {
        throw BareBones::Exception(0x7bcd6011e39bbabc, "Attempt to load snapshot into non-empty DecodingTable");
      } else if (first_to_load_i < size()) {
        throw BareBones::Exception(0x3387739a7b4f574a, "Attempt to load segment over existing DecodingTable data");
      } else {
        throw BareBones::Exception(0x4ece66e098927bc6,
                                   "Attempt to load incomplete data from segment, DecodingTable data vector length (%u) is less than segment size(%d)", size(),
                                   first_to_load_i);
      }
    }

    // read size
    uint32_t size_to_load = 0;
    in.read(reinterpret_cast<char*>(&size_to_load), sizeof(size_to_load));

    // read is completed if there are no items
    if (size_to_load == 0) {
      return;
    }

    auto storage_checkpoint = storage_.checkpoint();
    auto sg2 = std::experimental::scope_fail([&]() { storage_.rollback(storage_checkpoint); });
    const auto loaded_range = storage_.load(in, size_to_load, version);

    // post-processing
    if constexpr (has_after_items_load<Derived, decltype(loaded_range)>) {
      static_assert(noexcept(static_cast<Derived*>(this)->after_items_load_impl(loaded_range)));
      static_cast<Derived*>(this)->after_items_load_impl(loaded_range);
    }
  }

  template <OutputStream S>
  friend S& operator<<(S& out, GenericDecodingTable& decoding_table) {
    out << decoding_table.checkpoint();
    return out;
  }

  template <InputStream S>
  friend S& operator>>(S& in, GenericDecodingTable& decoding_table) {
    decoding_table.load(in);
    return in;
  }

  PROMPP_ALWAYS_INLINE auto begin() const noexcept { return storage_.view().begin(); }
  PROMPP_ALWAYS_INLINE auto end() const noexcept { return storage_.view().end(); }

  PROMPP_ALWAYS_INLINE auto remainder_size() const noexcept { return storage_.remainder_size(); }

  PROMPP_ALWAYS_INLINE auto version() const noexcept { return version_; }
};

template <template <template <class> class> class Filament, template <class> class Vector>
class DecodingTable : public GenericDecodingTable<DecodingTable<Filament, Vector>, Filament, Vector> {
 public:
  using Base = GenericDecodingTable<DecodingTable, Filament, Vector>;

  using Base::Base;
};

template <template <template <class> class> class Filament, template <class> class Vector>
  requires is_shrinkable<typename Filament<Vector>::storage_type>
class ShrinkableEncodingBimap final : private GenericDecodingTable<ShrinkableEncodingBimap<Filament, Vector>, Filament, Vector> {
 public:
  using Base = GenericDecodingTable<ShrinkableEncodingBimap, Filament, Vector>;
  using checkpoint_type = typename Base::checkpoint_type;
  using value_type = typename Base::value_type;

  using Base::checkpoint;
  using Base::load;
  using Base::next_item_index;
  using Base::remainder_size;
  using Base::size;

  friend class GenericDecodingTable<ShrinkableEncodingBimap, Filament, Vector>;

  template <class Class>
  PROMPP_ALWAYS_INLINE uint32_t find_or_emplace(const Class& c) noexcept {
    return *set_.lazy_emplace(c, [&](const auto& ctor) {
      const auto id = Base::storage_.emplace_back(c);
      ctor(id);
    }) + shift_;
  }

  template <class Class>
  PROMPP_ALWAYS_INLINE uint32_t find_or_emplace(const Class& c, size_t hashval) noexcept {
    return *set_.lazy_emplace_with_hash(c, phmap::phmap_mix<sizeof(size_t)>()(hashval), [&](const auto& ctor) {
      const auto id = Base::storage_.emplace_back(c);
      ctor(id);
    }) + shift_;
  }

  template <class Class>
  PROMPP_ALWAYS_INLINE std::optional<uint32_t> find(const Class& c) const noexcept {
    if (auto i = set_.find(c); i != set_.end()) {
      return *i + shift_;
    }
    return {};
  }

  template <class Class>
  PROMPP_ALWAYS_INLINE std::optional<uint32_t> find(const Class& c, size_t hashval) const noexcept {
    if (auto i = set_.find(c, phmap::phmap_mix<sizeof(size_t)>()(hashval)); i != set_.end()) {
      return *i + shift_;
    }
    return {};
  }

  void shrink_to_checkpoint_size(const checkpoint_type& checkpoint) {
    if (checkpoint.next_item_index() != next_item_index_impl()) {
      throw Exception(0x1bf0dbff9fe3d955, "Invalid checkpoint to shrink: checkpoint next_item_index [%u], next_item_index [%u]", checkpoint.next_item_index(),
                      next_item_index_impl());
    }

    shift_ += Base::size();
    Base::storage_.shrink();
    set_.clear();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return Base::allocated_memory() + set_allocated_memory_; }

  PROMPP_ALWAYS_INLINE value_type operator[](uint32_t id) const noexcept {
    assert(id >= shift_);
    return Base::operator[](id - shift_);
  }

  template <InputStream S>
  friend S& operator>>(S& in, ShrinkableEncodingBimap& shrinkable_encoding_bimap) {
    shrinkable_encoding_bimap.load(in);
    return in;
  }

 private:
  uint32_t set_allocated_memory_{};
  phmap::flat_hash_set<typename Base::Proxy, typename Base::Hasher, typename Base::EqualityComparator, Allocator<typename Base::Proxy, uint32_t>> set_{
      {},
      0,
      Base::hasher(),
      Base::equality_comparator(),
      Allocator<typename Base::Proxy, uint32_t>{set_allocated_memory_}};
  uint32_t shift_{0};

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t next_item_index_impl() const noexcept { return shift_ + Base::storage_.next_item_index(); }

  template <ls_id_range R>
  PROMPP_ALWAYS_INLINE void after_items_load_impl(R&& loaded_ids) noexcept {
    if constexpr (std::ranges::sized_range<R>) {
      set_.reserve(std::ranges::size(loaded_ids));
    }
    for (const auto id : loaded_ids) {
      set_.emplace(typename Base::Proxy(id));
    }
  }
};

template <template <template <class> class> class Filament, template <class> class Vector>
class EncodingBimap : public GenericDecodingTable<EncodingBimap<Filament, Vector>, Filament, Vector> {
  using Base = GenericDecodingTable<EncodingBimap, Filament, Vector>;

  friend class GenericDecodingTable<EncodingBimap, Filament, Vector>;

  phmap::flat_hash_set<typename Base::Proxy, typename Base::Hasher, typename Base::EqualityComparator> set_{{}, 0, Base::hasher(), Base::equality_comparator()};

  template <ls_id_range R>
  PROMPP_ALWAYS_INLINE void after_items_load_impl(R&& loaded_ids) noexcept {
    if constexpr (std::ranges::sized_range<R>) {
      set_.reserve(std::ranges::size(loaded_ids));
    }
    for (const auto id : loaded_ids) {
      set_.emplace(typename Base::Proxy(id));
    }
  }

 public:
  using Base::Base;
  EncodingBimap() noexcept = default;
  EncodingBimap(const EncodingBimap& other) = delete;
  EncodingBimap(EncodingBimap&&) noexcept = delete;
  EncodingBimap& operator=(const EncodingBimap&) = delete;
  EncodingBimap& operator=(EncodingBimap&&) noexcept = delete;

  template <class Class>
  PROMPP_ALWAYS_INLINE uint32_t find_or_emplace(const Class& c) noexcept {
    return *set_.lazy_emplace(c, [&](const auto& ctor) {
      const uint32_t id = Base::storage_.emplace_back(c);
      ctor(id);
    });
  }

  template <class Class>
  PROMPP_ALWAYS_INLINE uint32_t find_or_emplace(const Class& c, size_t hashval) noexcept {
    return *set_.lazy_emplace_with_hash(c, phmap::phmap_mix<sizeof(size_t)>()(hashval), [&](const auto& ctor) {
      const uint32_t id = Base::storage_.emplace_back(c);
      ctor(id);
    });
  }

  template <class Class, class Cache, class... Args>
  PROMPP_ALWAYS_INLINE uint32_t find_or_emplace_with_cache(const Class& c, uint32_t id, Cache& cache, Args&&... args) noexcept {
    if (const auto value = cache[id]; value != Base::kInvalidId) {
      return value;
    }

    return *set_.lazy_emplace(c, [&](const auto& ctor) {
      uint32_t new_id = Base::storage_.emplace_back(c, std::forward<Args>(args)...);
      ctor(new_id);
      cache[id] = new_id;
    });
  }

  template <class Class>
  PROMPP_ALWAYS_INLINE std::optional<uint32_t> find(const Class& c) const noexcept {
    if (auto i = set_.find(c); i != set_.end()) {
      return *i;
    }
    return {};
  }

  template <class Class>
  PROMPP_ALWAYS_INLINE std::optional<uint32_t> find(const Class& c, size_t hashval) const noexcept {
    if (auto i = set_.find(c, phmap::phmap_mix<sizeof(size_t)>()(hashval)); i != set_.end()) {
      return *i;
    }
    return {};
  }

  PROMPP_ALWAYS_INLINE void rollback_impl(const typename Base::checkpoint_type& s) noexcept
    requires(!Base::kIsReadOnly)
  {
    assert(s.size() <= Base::size());

    for (uint32_t i = s.size(); i != Base::size(); ++i) {
      set_.erase(typename Base::Proxy(i));
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    return Base::allocated_memory() + set_.capacity() * sizeof(typename Base::Proxy);
  }
};

}  // namespace BareBones::SnugComposite
