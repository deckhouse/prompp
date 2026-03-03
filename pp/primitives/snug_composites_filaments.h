#pragma once

#include <scope_exit.h>

#include "bare_bones/exception.h"
#include "bare_bones/iterator.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/snug_composite.h"
#include "bare_bones/stream_v_byte.h"
#include "hash.h"

namespace PromPP::Primitives::SnugComposites::Filaments {

template <template <class> class Vector>
struct Symbol {
  class storage_type {
   public:
    static constexpr bool kIsReadOnly = BareBones::IsSharedSpan<Vector<uint8_t>>::value;

    using composite_type = std::string_view;
    struct item_type {
      uint32_t pos;
      uint32_t length;
    };

    struct checkpoint_type {
      uint32_t data_size;
      uint32_t items_size;

      using SerializationMode = BareBones::SnugComposite::SerializationMode;

      template <BareBones::OutputStream S>
      void save(S& out, const storage_type& storage, uint32_t id_offset, uint32_t id_count, uint8_t table_version, checkpoint_type const* from = nullptr)
          const {
        if (table_version == 1) {
          // write items
          out.write(reinterpret_cast<const char*>(&storage.items_[id_offset]), sizeof(item_type) * id_count);

          // write a version
          out.put(1);
        } else {  // table_version == 2
          // write a version
          out.put(1);

          // write items
          out.write(reinterpret_cast<const char*>(&storage.items_[id_offset]), sizeof(item_type) * id_count);
        }

        // write mode
        SerializationMode mode = (from != nullptr) ? SerializationMode::DELTA : SerializationMode::SNAPSHOT;
        out.put(static_cast<char>(mode));

        // write pos of the first seq in the portion if we are writing delta
        uint32_t first_to_save = 0;
        if (from != nullptr) {
          first_to_save = from->data_size;
          out.write(reinterpret_cast<const char*>(&first_to_save), sizeof(first_to_save));
        }

        // write size
        uint32_t size_to_save = data_size - first_to_save;
        out.write(reinterpret_cast<char*>(&size_to_save), sizeof(size_to_save));

        // write data
        if (size_to_save > 0) {
          out.write(&storage.data_[first_to_save], size_to_save);
        }
      }

      uint32_t save_size(const storage_type&, uint32_t id_count, checkpoint_type const* from = nullptr) const {
        uint32_t res = 0;

        // items
        res += sizeof(item_type) * id_count;

        // version
        ++res;

        // mode
        ++res;

        // pos of first seq in the portion, if we are writing delta
        uint32_t first_to_save = 0;
        if (from != nullptr) {
          first_to_save = from->data_size;
          res += sizeof(uint32_t);  // first index
        }

        // size
        const uint32_t size_to_save = data_size - first_to_save;
        res += sizeof(uint32_t);

        // data
        res += size_to_save;

        return res;
      }
    };

    struct symbols_view {
      const storage_type* storage_ptr;

      class iterator_type {
       public:
        using iterator_category = std::input_iterator_tag;
        using value_type = composite_type;
        using difference_type = std::ptrdiff_t;

        iterator_type() = default;
        explicit iterator_type(const storage_type& storage, uint32_t id) noexcept : storage_ptr_(&storage), id_{id} {}

        PROMPP_ALWAYS_INLINE iterator_type& operator++() noexcept {
          ++id_;
          return *this;
        }

        PROMPP_ALWAYS_INLINE iterator_type operator++(int) noexcept {
          iterator_type retval = *this;
          ++(*this);
          return retval;
        }

        PROMPP_ALWAYS_INLINE bool operator==(const iterator_type& other) const noexcept { return id_ == other.id_; }
        PROMPP_ALWAYS_INLINE bool operator==(BareBones::iterator::IteratorSentinelType) const noexcept { return id_ == storage_ptr_->items_.size(); }

        [[nodiscard]] PROMPP_ALWAYS_INLINE value_type operator*() const noexcept { return storage_ptr_->composite(id_); }

        [[nodiscard]] uint32_t id() const noexcept { return id_; }

       private:
        const storage_type* storage_ptr_;
        uint32_t id_{0};
      };

      [[nodiscard]] PROMPP_ALWAYS_INLINE auto begin() const noexcept { return iterator_type{*storage_ptr, 0}; }
      [[nodiscard]] PROMPP_ALWAYS_INLINE auto end() const noexcept { return iterator_type{*storage_ptr, storage_ptr->items_.size()}; }

      [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return storage_ptr->count(); }
      [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t next_item_index() const noexcept { return storage_ptr->next_item_index(); }

      [[nodiscard]] PROMPP_ALWAYS_INLINE composite_type operator[](uint32_t id) const noexcept { return storage_ptr->composite(id); }
    };

    using view_type = symbols_view;

    storage_type() noexcept = default;
    template <class AnotherStorageType>
      requires kIsReadOnly
    explicit storage_type(const AnotherStorageType& other) : data_{other.data_}, items_{other.items_} {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t count() const noexcept { return static_cast<uint32_t>(items_.size()); }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t remainder_size() const noexcept {
      constexpr uint32_t max_ui32 = std::numeric_limits<uint32_t>::max();
      assert(data_.size() <= max_ui32);
      return max_ui32 - static_cast<uint32_t>(data_.size());
    }

    template <class AnotherStorageType>
    PROMPP_ALWAYS_INLINE void reserve(const AnotherStorageType& other) noexcept {
      items_.reserve(other.items_.size());
      data_.reserve(other.data_.size());
    }

    PROMPP_ALWAYS_INLINE void reserve(uint32_t count) noexcept { items_.reserve(count); }

    [[nodiscard]] PROMPP_ALWAYS_INLINE composite_type composite(uint32_t id) const noexcept {
      const auto item = items_[id];
      return std::string_view(data_.data() + item.pos, item.length);
    }

    void validate(uint32_t id) const {
      if (const auto item = items_[id]; item.pos + item.length > data_.size()) {
        throw BareBones::Exception(0x75555f55ebe357a3, "Symbol validation error: length is out of data vector range");
      }
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t allocated_memory() const noexcept {
      return BareBones::mem::allocated_memory(data_) + BareBones::mem::allocated_memory(items_);
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t next_item_index() const noexcept { return static_cast<uint32_t>(items_.size()); }

    PROMPP_ALWAYS_INLINE uint32_t emplace_back(composite_type str) noexcept {
      const auto id = static_cast<uint32_t>(items_.size());
      items_.emplace_back(static_cast<uint32_t>(data_.size()), static_cast<uint32_t>(str.length()));
      data_.push_back(str.begin(), str.end());
      return id;
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE checkpoint_type checkpoint() const noexcept {
      return {static_cast<uint32_t>(data_.size()), static_cast<uint32_t>(items_.size())};
    }
    PROMPP_ALWAYS_INLINE void rollback(const checkpoint_type& checkpoint) noexcept {
      assert(checkpoint.data_size <= data_.size());
      assert(checkpoint.items_size <= items_.size());
      data_.resize(checkpoint.data_size);
      items_.resize(checkpoint.items_size);
    }

    template <class InputStream>
    auto load(InputStream& in, uint32_t items_size, uint8_t table_version) {
      const auto original_size = items_.size();
      if (table_version == 1) {
        // read items
        items_.resize(original_size + items_size);
        in.read(reinterpret_cast<char*>(&items_[original_size]), sizeof(item_type) * items_size);
      }

      // read version
      const uint8_t version = in.get();
      if (version != 1) {
        throw BareBones::Exception(0x67c010edbd64e272,
                                   "Invalid stream data version (%d) for loading into data vector (Symbol::data_type), only version 1 is supported", version);
      }

      if (table_version == 2) {
        // read items
        items_.resize(original_size + items_size);
        in.read(reinterpret_cast<char*>(&items_[original_size]), sizeof(item_type) * items_size);
      }

      // read mode
      const auto mode = static_cast<BareBones::SnugComposite::SerializationMode>(in.get());

      // read pos of the first symbol in the portion if we are reading wal
      uint32_t first_to_load_i = 0;
      if (mode == BareBones::SnugComposite::SerializationMode::DELTA) {
        in.read(reinterpret_cast<char*>(&first_to_load_i), sizeof(first_to_load_i));
      }

      if (first_to_load_i != data_.size()) {
        if (mode == BareBones::SnugComposite::SerializationMode::SNAPSHOT) {
          throw BareBones::Exception(0x4c0ca0586da6da3f, "Attempt to load snapshot into non-empty data vector");
        } else if (first_to_load_i < data_.size()) {
          throw BareBones::Exception(0x55cb9b02c23f7bbd, "Attempt to load segment over existing data");
        } else {
          throw BareBones::Exception(0x55cb9b02c23f7bbd, "Attempt to load incomplete data from segment, data vector length (%u) is less than segment size (%d)",
                                     data_.size(), first_to_load_i);
        }
      }

      // read size
      uint32_t size_to_load = 0;
      in.read(reinterpret_cast<char*>(&size_to_load), sizeof(size_to_load));

      // read data
      data_.resize(data_.size() + size_to_load);
      in.read(data_.begin() + first_to_load_i, size_to_load);

      return std::views::iota(original_size, items_.size());
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE view_type view() const noexcept { return {.storage_ptr = this}; }

   private:
    template <template <class> class>
    friend struct Symbol;

    Vector<char> data_;
    Vector<item_type> items_;
  };
};

}  // namespace PromPP::Primitives::SnugComposites::Filaments

template <template <class> class Vector>
struct BareBones::IsTriviallyReallocatable<BareBones::SnugComposite::DecodingTable<PromPP::Primitives::SnugComposites::Filaments::Symbol, Vector>>
    : std::true_type {};

namespace PromPP::Primitives::SnugComposites::Filaments {

template <class Iterator>
concept has_id = requires(Iterator it) {
  { it.id() };
};

template <class Iterator>
concept has_name_id = requires(Iterator it) {
  { it.name_id() };
};

template <class Iterator>
concept has_id_or_name_id = has_id<Iterator> || has_name_id<Iterator>;

template <class Table, class Item, class Cache>
concept has_find_or_emplace_with_cache = requires(Table table, Item item, Cache cache) {
  { table.find_or_emplace_with_cache(item, uint32_t(), cache) };
};

struct NoCache {};

template <class Cache, class Iterator, class Table, class Item>
concept use_find_or_emplace_with_cache =
    !std::same_as<Cache, NoCache> && has_id_or_name_id<Iterator> && has_find_or_emplace_with_cache<Table, Item, typename std::remove_cvref_t<Cache>::ItemList>;

template <template <template <class> class> class SymbolsTableType, template <class> class Vector>
struct LabelNameSet {
  class storage_type {
   public:
    static constexpr bool kIsReadOnly = BareBones::IsSharedSpan<Vector<uint8_t>>::value;

    using symbols_table_type = SymbolsTableType<Vector>;
    using symbols_ids_sequences_type = Vector<uint32_t>;

    struct item_type {
      uint32_t pos;
      uint32_t size;
    };

    class composite_type {
     public:
      class iterator_type {
       public:
        using iterator_category = std::input_iterator_tag;
        using value_type = symbols_table_type::value_type;
        using difference_type = std::ptrdiff_t;

        iterator_type() = default;
        explicit iterator_type(const symbols_table_type* symbols_table_ptr, symbols_ids_sequences_type::const_iterator it) noexcept
            : symbols_table_ptr_{symbols_table_ptr}, symbols_ids_it_{it} {}

        PROMPP_ALWAYS_INLINE iterator_type& operator++() noexcept {
          ++symbols_ids_it_;
          return *this;
        }

        PROMPP_ALWAYS_INLINE iterator_type operator++(int) noexcept {
          iterator_type retval = *this;
          ++(*this);
          return retval;
        }
        PROMPP_ALWAYS_INLINE bool operator==(const iterator_type& other) const noexcept { return symbols_ids_it_ == other.symbols_ids_it_; }

        [[nodiscard]] PROMPP_ALWAYS_INLINE value_type operator*() const noexcept { return symbols_table_ptr_->operator[](*symbols_ids_it_); }

        [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t id() const noexcept { return *symbols_ids_it_; }

       private:
        const symbols_table_type* symbols_table_ptr_;
        symbols_ids_sequences_type::const_iterator symbols_ids_it_;
      };
      using value_type = iterator_type::value_type;

      composite_type() = default;
      explicit composite_type(const symbols_table_type* symbols_table_ptr, symbols_ids_sequences_type::const_iterator symbols_ids_begin, uint32_t symbols_count)
          : symbols_table_ptr_{symbols_table_ptr}, symbols_ids_begin_{symbols_ids_begin}, symbols_count_{symbols_count} {}

      [[nodiscard]] PROMPP_ALWAYS_INLINE auto begin() const noexcept { return iterator_type{symbols_table_ptr_, symbols_ids_begin_}; }
      [[nodiscard]] PROMPP_ALWAYS_INLINE auto end() const noexcept { return iterator_type{symbols_table_ptr_, symbols_ids_begin_ + size()}; }

      [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return symbols_count_; }

      template <class T>
      PROMPP_ALWAYS_INLINE bool operator==(const T& other) const noexcept {
        return std::ranges::equal(begin(), end(), other.begin(), other.end());
      }

      template <class T>
      PROMPP_ALWAYS_INLINE bool operator<(const T& other) const noexcept {
        return std::ranges::lexicographical_compare(begin(), end(), other.begin(), other.end());
      }

      friend size_t hash_value(const composite_type& lns) noexcept { return hash::hash_of_string_list(lns); }

     private:
      const symbols_table_type* symbols_table_ptr_;
      symbols_ids_sequences_type::const_iterator symbols_ids_begin_;
      uint32_t symbols_count_{};
    };

    struct checkpoint_type {
      symbols_table_type::checkpoint_type symbols_table_checkpoint;
      uint32_t items_size;
      uint32_t symbols_ids_size;

      using SerializationMode = BareBones::SnugComposite::SerializationMode;

      template <BareBones::OutputStream S>
      void save(S& out, const storage_type& storage, uint32_t id_offset, uint32_t id_count, uint8_t table_version, checkpoint_type const* from = nullptr)
          const {
        if (table_version == 1) {
          // write items
          out.write(reinterpret_cast<const char*>(&storage.items_[id_offset]), sizeof(item_type) * id_count);

          // write a version
          out.put(1);
        } else {  // table_version == 2
          // write a version
          out.put(1);

          // write items
          out.write(reinterpret_cast<const char*>(&storage.items_[id_offset]), sizeof(item_type) * id_count);
        }

        // write mode
        SerializationMode mode = (from != nullptr) ? SerializationMode::DELTA : SerializationMode::SNAPSHOT;
        out.put(static_cast<char>(mode));

        // write pos of the first seq in the portion if we are writing delta
        uint32_t first_to_save = 0;
        if (from != nullptr) {
          first_to_save = from->symbols_ids_size;
          out.write(reinterpret_cast<const char*>(&first_to_save), sizeof(first_to_save));
        }

        // write size
        uint32_t size_to_save = symbols_ids_size - first_to_save;
        out.write(reinterpret_cast<char*>(&size_to_save), sizeof(size_to_save));

        // write symbols ids
        out.write(reinterpret_cast<const char*>(&storage.symbols_ids_sequences_[first_to_save]),
                  sizeof(storage.symbols_ids_sequences_[first_to_save]) * size_to_save);

        // write symbols table
        if (from != nullptr) {
          symbols_table_checkpoint.save(out, &from->symbols_table_checkpoint);
        } else {
          symbols_table_checkpoint.save(out);
        }
      }

      uint32_t save_size(const storage_type& storage, uint32_t id_count, checkpoint_type const* from = nullptr) const {
        uint32_t res = 0;

        // items
        res += sizeof(item_type) * id_count;

        // version
        ++res;

        // mode
        ++res;

        // pos of first seq in the portion, if we are writing delta
        uint32_t first_to_save = 0;
        if (from != nullptr) {
          first_to_save = from->symbols_ids_size;
          res += sizeof(uint32_t);
        }

        // size
        const uint32_t size_to_save = symbols_ids_size - first_to_save;
        res += sizeof(uint32_t);

        // data
        res += sizeof(storage.symbols_ids_sequences_[first_to_save]) * size_to_save;

        // symbols table
        if (from != nullptr) {
          res += symbols_table_checkpoint.save_size(&from->symbols_table_checkpoint);
        } else {
          res += symbols_table_checkpoint.save_size();
        }

        return res;
      }
    };

    struct label_name_set_view {
      using symbols_view_type = symbols_table_type::storage_type::view_type;
      const storage_type* storage_ptr;

      class iterator_type {
       public:
        using iterator_category = std::input_iterator_tag;
        using value_type = composite_type;
        using difference_type = std::ptrdiff_t;

        iterator_type() = default;
        explicit iterator_type(const storage_type* storage_ptr, uint32_t id) noexcept : storage_ptr_(storage_ptr), id_{id} {}

        PROMPP_ALWAYS_INLINE iterator_type& operator++() noexcept {
          ++id_;
          return *this;
        }

        PROMPP_ALWAYS_INLINE iterator_type operator++(int) noexcept {
          iterator_type retval = *this;
          ++(*this);
          return retval;
        }
        PROMPP_ALWAYS_INLINE bool operator==(const iterator_type& other) const noexcept { return id_ == other.id_; }
        PROMPP_ALWAYS_INLINE bool operator==(BareBones::iterator::IteratorSentinelType) const noexcept { return id_ == storage_ptr_->items_.size(); }

        [[nodiscard]] PROMPP_ALWAYS_INLINE value_type operator*() const noexcept { return storage_ptr_->composite(id_); }

        [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t id() const noexcept { return id_; }

       private:
        const storage_type* storage_ptr_;
        uint32_t id_{0};
      };

      [[nodiscard]] PROMPP_ALWAYS_INLINE auto begin() const noexcept { return iterator_type{storage_ptr, 0}; }
      [[nodiscard]] PROMPP_ALWAYS_INLINE auto end() const noexcept { return iterator_type{storage_ptr, storage_ptr->items_.size()}; }

      [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return storage_ptr->count(); }
      [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t next_item_index() const noexcept { return storage_ptr->next_item_index(); }

      [[nodiscard]] PROMPP_ALWAYS_INLINE symbols_view_type symbols() const noexcept { return storage_ptr->symbols_table_.data_view(); }
    };

    using view_type = label_name_set_view;

    storage_type() noexcept = default;
    template <class AnotherStorageType>
      requires kIsReadOnly
    explicit storage_type(const AnotherStorageType& other)
        : symbols_table_{other.symbols_table_}, symbols_ids_sequences_{other.symbols_ids_sequences_}, items_{other.items_} {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t count() const noexcept { return static_cast<uint32_t>(items_.size()); }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t remainder_size() const noexcept {
      constexpr uint32_t max_ui32 = std::numeric_limits<uint32_t>::max();
      assert(symbols_ids_sequences_.size() <= max_ui32);

      const uint32_t remainder_for_symbols = max_ui32 - static_cast<uint32_t>(symbols_ids_sequences_.size());
      return std::min(symbols_table_.remainder_size(), remainder_for_symbols);
    }

    template <class AnotherStorageType>
    PROMPP_ALWAYS_INLINE void reserve(const AnotherStorageType& other) noexcept {
      symbols_table_.reserve(other.symbols_table_);
      symbols_ids_sequences_.reserve(other.symbols_ids_sequences_.size());
      items_.reserve(other.items_.size());
    }

    PROMPP_ALWAYS_INLINE void reserve(uint32_t count) noexcept { items_.reserve(count); }

    [[nodiscard]] PROMPP_ALWAYS_INLINE composite_type composite(uint32_t id) const noexcept {
      const auto item = items_[id];
      const auto symbols_ids_begin = symbols_ids_sequences_.begin() + item.pos;
      return composite_type(&symbols_table_, symbols_ids_begin, item.size);
    }

    void validate(uint32_t id) const {
      const auto [pos, size] = items_[id];

      if (pos + size > symbols_ids_sequences_.size()) {
        throw BareBones::Exception(0x45e8bdc1455fd8e4, "LabelSetNames data validation error: expected LabelSetNames length is out of data vector range");
      }

      for (auto i = symbols_ids_sequences_.begin() + pos; i != symbols_ids_sequences_.begin() + pos + size; ++i) {
        if (*i >= symbols_table_.size()) {
          throw BareBones::Exception(0x218410dde097cc6b,
                                     "LabelSetNames data validation error: expected LabelSetNames length is out of data symbols table range");
        }
      }
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t allocated_memory() const noexcept {
      return BareBones::mem::allocated_memory(symbols_table_) + BareBones::mem::allocated_memory(symbols_ids_sequences_) +
             BareBones::mem::allocated_memory(items_);
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t next_item_index() const noexcept { return static_cast<uint32_t>(items_.size()); }

    template <class OtherLabelNameSet, class Cache = NoCache>
    PROMPP_ALWAYS_INLINE uint32_t emplace_back(const OtherLabelNameSet& label_name_set, Cache&& cache = {}) noexcept {
      const auto id = static_cast<uint32_t>(items_.size());

      auto pos = static_cast<uint32_t>(symbols_ids_sequences_.size());
      uint32_t size = 0;

      if constexpr (BareBones::concepts::has_size<OtherLabelNameSet>) {
        size = label_name_set.size();
        symbols_ids_sequences_.reserve(static_cast<size_t>(symbols_ids_sequences_.size()) + size);
      }

      const auto end = label_name_set.end();
      for (auto it = label_name_set.begin(); it != end; ++it) {
        symbols_ids_sequences_.push_back(find_or_emplace_label_name(it, std::forward<Cache>(cache)));
        if constexpr (!BareBones::concepts::has_size<OtherLabelNameSet>) {
          ++size;
        }
      }

      items_.emplace_back(pos, size);

      return id;
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE checkpoint_type checkpoint() const noexcept {
      return {symbols_table_.checkpoint(), static_cast<uint32_t>(items_.size()), static_cast<uint32_t>(symbols_ids_sequences_.size())};
    }

    PROMPP_ALWAYS_INLINE void rollback(const checkpoint_type& checkpoint) noexcept {
      assert(checkpoint.symbols_ids_size <= symbols_ids_sequences_.size());
      assert(checkpoint.items_size <= items_.size());

      symbols_ids_sequences_.resize(checkpoint.symbols_ids_size);
      symbols_table_.rollback(checkpoint.symbols_table_checkpoint);
      items_.resize(checkpoint.items_size);
    }

    template <class InputStream>
    auto load(InputStream& in, uint32_t items_size, uint8_t table_version) {
      const auto items_original_size = items_.size();
      if (table_version == 1) {
        // read items
        items_.resize(items_original_size + items_size);
        in.read(reinterpret_cast<char*>(&items_[items_original_size]), sizeof(item_type) * items_size);
      }

      // read version
      const uint8_t version = in.get();
      if (version != 1) {
        throw BareBones::Exception(0xe7b943f626c40350,
                                   "Invalid stream data version (%d) for loading into LabelSetNames::data_type vector, only version 1 is supported", version);
      }

      if (table_version == 2) {
        // read items
        items_.resize(items_original_size + items_size);
        in.read(reinterpret_cast<char*>(&items_[items_original_size]), sizeof(item_type) * items_size);
      }

      // read mode
      const auto mode = static_cast<BareBones::SnugComposite::SerializationMode>(in.get());

      // read pos of first seq in the portion if we are reading wal
      uint32_t first_to_load_i = 0;
      if (mode == BareBones::SnugComposite::SerializationMode::DELTA) {
        in.read(reinterpret_cast<char*>(&first_to_load_i), sizeof(first_to_load_i));
      }
      if (first_to_load_i != symbols_ids_sequences_.size()) {
        if (mode == BareBones::SnugComposite::SerializationMode::SNAPSHOT) {
          throw BareBones::Exception(0x484607065485b4ab, "Attempt to load snapshot into non-empty LabelSetNames data vector");
        } else if (first_to_load_i < symbols_ids_sequences_.size()) {
          throw BareBones::Exception(0xc042fdcb4b149d95, "Attempt to load segment over existing LabelSetNames data");
        } else {
          throw BareBones::Exception(0x79995816e0a9690b,
                                     "Attempt to load incomplete data from segment, LabelSetNames data vector length (%u) is less than segment size (%d)",
                                     symbols_ids_sequences_.size(), first_to_load_i);
        }
      }

      // read size
      uint32_t size_to_load = 0;
      in.read(reinterpret_cast<char*>(&size_to_load), sizeof(size_to_load));

      // read data
      auto original_size = symbols_ids_sequences_.size();
      auto sg1 = std::experimental::scope_fail([&]() { symbols_ids_sequences_.resize(original_size); });
      symbols_ids_sequences_.resize(original_size + size_to_load);
      in.read(reinterpret_cast<char*>(&symbols_ids_sequences_[first_to_load_i]), sizeof(symbols_ids_sequences_[first_to_load_i]) * size_to_load);

      // read symbols table
      symbols_table_.load(in);

      return std::views::iota(items_original_size, items_.size());
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE view_type view() const noexcept { return {.storage_ptr = this}; }

   private:
    template <template <template <class> class> class, template <class> class>
    friend struct LabelNameSet;

    symbols_table_type symbols_table_;
    symbols_ids_sequences_type symbols_ids_sequences_;
    Vector<item_type> items_;

    template <class LabelNameIterator, class Cache>
    PROMPP_ALWAYS_INLINE uint32_t find_or_emplace_label_name(const LabelNameIterator& label_name, Cache&& cache) {
      if constexpr (use_find_or_emplace_with_cache<Cache, LabelNameIterator, symbols_table_type, decltype(*label_name)>) {
        symbols_table_.find_or_emplace_with_cache(*label_name, label_name.id(), cache.names);
      }

      return symbols_table_.find_or_emplace(*label_name);
    }
  };
};

template <template <template <class> class> class SymbolsTableType,
          template <template <class> class> class LabelNameSetsTableType,
          template <class> class Vector>
struct LabelSet {
  class storage_type {
   public:
    static constexpr bool kIsReadOnly = BareBones::IsSharedSpan<Vector<uint8_t>>::value;

    using label_values_symbols_table_type = SymbolsTableType<Vector>;
    using label_name_sets_table_type = LabelNameSetsTableType<Vector>;

    using symbols_tables_type = BareBones::Vector<label_values_symbols_table_type>;

    using symbol_ids_codec_type = BareBones::StreamVByte::Codec1234;
    using symbols_ids_sequences_type = Vector<uint8_t>;

    class composite_type {
      using label_name_set_type = label_name_sets_table_type::value_type;
      using values_iterator_type = BareBones::StreamVByte::DecodeIterator<symbol_ids_codec_type, typename symbols_ids_sequences_type::const_iterator>;
      using names_iterator_type = label_name_set_type::iterator_type;

      label_name_set_type label_name_set_;
      const symbols_tables_type* symbols_tables_ptr_;
      values_iterator_type values_begin_;
      uint32_t name_set_id_;

     public:
      PROMPP_ALWAYS_INLINE explicit composite_type(const symbols_tables_type* symbols_tables_ptr = nullptr,
                                                   label_name_set_type label_name_set = label_name_set_type(),
                                                   values_iterator_type values_begin = values_iterator_type(),
                                                   uint32_t name_set_id = 0) noexcept
          : label_name_set_(label_name_set), symbols_tables_ptr_(symbols_tables_ptr), values_begin_(values_begin), name_set_id_(name_set_id) {}

      using value_type = std::pair<typename label_name_set_type::value_type, typename Symbol<Vector>::storage_type::composite_type>;

      [[nodiscard]] PROMPP_ALWAYS_INLINE const label_name_set_type& names() const noexcept { return label_name_set_; }

      [[nodiscard]] PROMPP_ALWAYS_INLINE auto size() const noexcept { return label_name_set_.size(); }
      [[nodiscard]] PROMPP_ALWAYS_INLINE auto id() const noexcept { return name_set_id_; }

      class iterator_type {
        names_iterator_type names_it_;
        values_iterator_type values_it_;
        const symbols_tables_type* symbols_tables_ptr_;

        friend class composite_type;

       public:
        using iterator_category = std::forward_iterator_tag;
        using value_type = composite_type::value_type;
        using difference_type = std::ptrdiff_t;

        PROMPP_ALWAYS_INLINE explicit iterator_type(const symbols_tables_type* symbols_tables_ptr = nullptr,
                                                    names_iterator_type names_it = names_iterator_type(),
                                                    values_iterator_type values_it = values_iterator_type()) noexcept
            : names_it_(names_it), values_it_(values_it), symbols_tables_ptr_(symbols_tables_ptr) {}

        PROMPP_ALWAYS_INLINE iterator_type& operator++() noexcept {
          ++names_it_;
          ++values_it_;
          return *this;
        }

        PROMPP_ALWAYS_INLINE iterator_type operator++(int) noexcept {
          iterator_type retval = *this;
          ++(*this);
          return retval;
        }

        PROMPP_ALWAYS_INLINE bool operator==(const iterator_type& other) const noexcept { return values_it_ == other.values_it_; }

        [[nodiscard]] PROMPP_ALWAYS_INLINE value_type operator*() const noexcept {
          if constexpr (BareBones::concepts::is_dereferenceable<typename symbols_tables_type::value_type>) {
            const auto& symbols_table = *(*symbols_tables_ptr_)[names_it_.id()];
            return {*names_it_, symbols_table[*values_it_]};
          } else {
            const auto& symbols_table = (*symbols_tables_ptr_)[names_it_.id()];
            return {*names_it_, symbols_table[*values_it_]};
          }
        }

        [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t name_id() const noexcept { return names_it_.id(); }
        [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t value_id() const noexcept { return *values_it_; }
      };

      [[nodiscard]] PROMPP_ALWAYS_INLINE auto begin() const noexcept { return iterator_type(symbols_tables_ptr_, label_name_set_.begin(), values_begin_); }
      [[nodiscard]] PROMPP_ALWAYS_INLINE auto end() const noexcept { return iterator_type(symbols_tables_ptr_, label_name_set_.end(), values_iterator_type()); }

      template <class T>
      PROMPP_ALWAYS_INLINE bool operator==(const T& b) const noexcept {
        return std::ranges::equal(begin(), end(), b.begin(), b.end(), [](const auto& a, const auto& b) { return a == b; });
      }

      template <class T>
      PROMPP_ALWAYS_INLINE bool operator<(const T& b) const noexcept {
        return std::ranges::lexicographical_compare(begin(), end(), b.begin(), b.end(), [](const auto& a, const auto& b) { return a < b; });
      }

      PROMPP_ALWAYS_INLINE friend size_t hash_value(const composite_type& ls) noexcept { return hash::hash_of_label_set(ls); }
    };

    struct item_type {
      uint32_t lns_id;
      uint32_t pos;
    };

    struct checkpoint_type {
      using SerializationMode = BareBones::SnugComposite::SerializationMode;
      using symbols_checkpoints_type = Vector<typename label_values_symbols_table_type::checkpoint_type>;

      label_name_sets_table_type::checkpoint_type label_name_sets_table_checkpoint_;
      symbols_checkpoints_type symbols_tables_checkpoints_;
      uint32_t next_item_index_;
      uint32_t size_;
      uint32_t items_size;

      template <BareBones::OutputStream S>
      void save(S& out, const storage_type& storage, uint32_t id_offset, uint32_t id_count, uint8_t table_version, checkpoint_type const* from = nullptr)
          const {
        if (table_version == 1) {
          // write items
          out.write(reinterpret_cast<const char*>(&storage.items_[id_offset]), sizeof(item_type) * id_count);

          // write a version
          out.put(1);
        } else {  // table_version == 2
          // write a version
          out.put(1);

          // write items
          out.write(reinterpret_cast<const char*>(&storage.items_[id_offset]), sizeof(item_type) * id_count);
        }

        // write mode
        SerializationMode mode = (from != nullptr) ? SerializationMode::DELTA : SerializationMode::SNAPSHOT;
        out.put(static_cast<char>(mode));

        // write pos of first seq in the portion, if we are writing delta
        uint32_t first_to_save = 0;
        if (from != nullptr) {
          first_to_save = from->next_item_index_;
          out.write(reinterpret_cast<const char*>(&first_to_save), sizeof(first_to_save));
        }
        const uint32_t first_item_index_in_ids_sequence = next_item_index_ - size_;
        assert(first_to_save >= first_item_index_in_ids_sequence);

        // write  size
        uint32_t size_to_save = next_item_index_ - first_to_save;
        out.write(reinterpret_cast<char*>(&size_to_save), sizeof(size_to_save));

        // write data
        out.write(reinterpret_cast<const char*>(&storage.symbols_ids_sequences_[first_to_save - first_item_index_in_ids_sequence]),
                  sizeof(storage.symbols_ids_sequences_[0]) * size_to_save);

        // write label name sets table
        if (from != nullptr) {
          label_name_sets_table_checkpoint_.save(out, &from->label_name_sets_table_checkpoint_);
        } else {
          label_name_sets_table_checkpoint_.save(out);
        }

        // count tables, we have to write
        uint32_t number_of_symbols_tables_to_save = symbols_tables_checkpoints_.size();
        if (from != nullptr) {
          for (uint32_t i = 0; i < from->symbols_tables_checkpoints_.size(); ++i) {
            auto from_checkpoint = from->symbols_tables_checkpoints_[i];
            auto to_checkpoint = symbols_tables_checkpoints_[i];
            if ((to_checkpoint - from_checkpoint).empty()) {
              --number_of_symbols_tables_to_save;
            }
          }
        }

        // write number of symbols tables
        out.write(reinterpret_cast<char*>(&number_of_symbols_tables_to_save), sizeof(number_of_symbols_tables_to_save));
        // write symbols tables
        if (from != nullptr) {
          for (uint32_t i = 0; i < symbols_tables_checkpoints_.size(); ++i) {
            auto to_checkpoint = symbols_tables_checkpoints_[i];
            if (i >= from->symbols_tables_checkpoints_.size()) {
              // write id
              out.write(reinterpret_cast<char*>(&i), sizeof(i));
              // write symbols table
              to_checkpoint.save(out);
              continue;
            }
            auto from_checkpoint = from->symbols_tables_checkpoints_[i];
            if ((to_checkpoint - from_checkpoint).empty()) {
              continue;
            }
            // write id
            out.write(reinterpret_cast<char*>(&i), sizeof(i));
            // write symbols table
            to_checkpoint.save(out, &from_checkpoint);
          }
        } else {
          for (uint32_t i = 0; i < symbols_tables_checkpoints_.size(); ++i) {
            // write symbols table
            symbols_tables_checkpoints_[i].save(out);
          }
        }
      }

      uint32_t save_size(const storage_type& storage, uint32_t id_count, checkpoint_type const* from = nullptr) const {
        uint32_t res = 0;

        // items
        res += sizeof(item_type) * id_count;

        // version
        ++res;

        // mode
        ++res;

        // pos of first seq in the portion, if we are writing wal
        uint32_t first_to_save = 0;
        if (from != nullptr) {
          first_to_save = from->next_item_index_;
          res += sizeof(uint32_t);
        }

        // size
        const uint32_t size_to_save = next_item_index_ - first_to_save;
        res += sizeof(uint32_t);

        // data
        res += sizeof(storage.symbols_ids_sequences_[0]) * size_to_save;

        // label name sets table
        if (from != nullptr) {
          res += label_name_sets_table_checkpoint_.save_size(&from->label_name_sets_table_checkpoint_);
        } else {
          res += label_name_sets_table_checkpoint_.save_size();
        }

        // number of symbols tables
        res += sizeof(uint32_t);

        // symbols tables
        if (from != nullptr) {
          for (uint32_t i = 0; i < symbols_tables_checkpoints_.size(); ++i) {
            const typename label_values_symbols_table_type::checkpoint_type* from_checkpoint = nullptr;
            if (i < from->symbols_tables_checkpoints_.size()) {
              from_checkpoint = &from->symbols_tables_checkpoints_[i];
            }
            auto to_checkpoint = symbols_tables_checkpoints_[i];
            if (from_checkpoint != nullptr) {
              if ((to_checkpoint - *from_checkpoint).empty()) {
                continue;
              }
            }
            // write id
            res += sizeof(i);
            // write symbols table
            res += to_checkpoint.save_size(from_checkpoint);
          }
        } else {
          for (uint32_t i = 0; i < symbols_tables_checkpoints_.size(); ++i) {
            // write symbols table
            res += symbols_tables_checkpoints_[i].save_size();
          }
        }

        return res;
      }
    };

    struct label_sets_values_view {
      using keys_view_type = label_name_sets_table_type::filament_type::storage_type::view_type::symbols_view_type;
      using values_symbols_view_type = label_values_symbols_table_type::filament_type::storage_type::view_type;

      const storage_type* storage_ptr;

      class iterator_type {
       public:
        using iterator_category = std::input_iterator_tag;
        using value_type = label_values_symbols_table_type::value_type;
        using difference_type = std::ptrdiff_t;

        iterator_type() = default;
        explicit iterator_type(const symbols_tables_type& symbols_tables,
                               const keys_view_type::iterator_type& key_it,
                               const keys_view_type::iterator_type& key_it_end) noexcept
            : symbols_tables_ptr_{&symbols_tables}, key_it_{key_it}, key_it_end_(key_it_end) {
          get_values_range();
        }

        PROMPP_ALWAYS_INLINE iterator_type& operator++() noexcept {
          ++value_it_;
          if (value_it_ == value_it_end_) {
            ++key_it_;
            get_values_range();
          }
          return *this;
        }

        PROMPP_ALWAYS_INLINE iterator_type operator++(int) noexcept {
          iterator_type retval = *this;
          ++(*this);
          return retval;
        }

        PROMPP_ALWAYS_INLINE bool operator==(const iterator_type& other) const = default;
        PROMPP_ALWAYS_INLINE bool operator==(BareBones::iterator::IteratorSentinelType) const noexcept {
          return key_it_ == key_it_end_ && value_it_ == value_it_end_;
        }

        [[nodiscard]] PROMPP_ALWAYS_INLINE value_type operator*() const noexcept { return *value_it_; }

        [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t key_id() const noexcept { return key_it_.id(); }
        [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t value_id() const noexcept { return value_it_.id(); }

       private:
        void get_values_range() noexcept {
          value_it_ = {};
          value_it_end_ = {};
          while (key_it_ != key_it_end_) {
            if constexpr (BareBones::concepts::is_dereferenceable<typename symbols_tables_type::value_type>) {
              const auto values_view = (*(*symbols_tables_ptr_)[key_it_.id()]).data_view();
              value_it_ = values_view.begin();
              value_it_end_ = values_view.end();
            } else {
              const auto values_view = (*symbols_tables_ptr_)[key_it_.id()].data_view();
              value_it_ = values_view.begin();
              value_it_end_ = values_view.end();
            }

            if (value_it_ != value_it_end_)
              return;

            ++key_it_;
          }
        }

        const symbols_tables_type* symbols_tables_ptr_;

        keys_view_type::iterator_type key_it_;
        keys_view_type::iterator_type key_it_end_;

        values_symbols_view_type::iterator_type value_it_;
        values_symbols_view_type::iterator_type value_it_end_;
      };

      [[nodiscard]] PROMPP_ALWAYS_INLINE auto begin() const noexcept {
        return iterator_type{storage_ptr->symbols_tables_, storage_ptr->label_name_sets_table_.data_view().symbols().begin(),
                             storage_ptr->label_name_sets_table_.data_view().symbols().end()};
      }
      [[nodiscard]] PROMPP_ALWAYS_INLINE auto end() const noexcept {
        return iterator_type{storage_ptr->symbols_tables_, storage_ptr->label_name_sets_table_.data_view().symbols().end(),
                             storage_ptr->label_name_sets_table_.data_view().symbols().end()};
      }

      [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size() const noexcept {
        size_t total_size = 0;
        const auto keys_view = storage_ptr->label_name_sets_table_.data_view().symbols();
        for (auto key_it = keys_view.begin(); key_it != keys_view.end(); ++key_it) {
          const uint32_t key_id = key_it.id();
          if constexpr (BareBones::concepts::is_dereferenceable<typename symbols_tables_type::value_type>) {
            total_size += (*storage_ptr->symbols_tables_[key_id]).data_view().size();
          } else {
            total_size += storage_ptr->symbols_tables_[key_id].data_view().size();
          }
        }
        return total_size;
      }
    };

    struct label_set_view {
      using keys_view_type = label_name_sets_table_type::filament_type::storage_type::view_type::symbols_view_type;
      using values_view_type = label_sets_values_view;
      const storage_type* storage_ptr;

      class iterator_type {
       public:
        using iterator_category = std::input_iterator_tag;
        using value_type = composite_type;
        using difference_type = std::ptrdiff_t;

        iterator_type() = default;
        explicit iterator_type(const storage_type& storage, uint32_t id) noexcept : storage_ptr_(&storage), id_{id} {}

        PROMPP_ALWAYS_INLINE iterator_type& operator++() noexcept {
          ++id_;
          return *this;
        }

        PROMPP_ALWAYS_INLINE iterator_type operator++(int) noexcept {
          iterator_type retval = *this;
          ++(*this);
          return retval;
        }

        PROMPP_ALWAYS_INLINE bool operator==(const iterator_type& other) const noexcept { return id_ == other.id_; }
        PROMPP_ALWAYS_INLINE bool operator==(BareBones::iterator::IteratorSentinelType) const noexcept { return id_ == storage_ptr_->items_.size(); }

        [[nodiscard]] PROMPP_ALWAYS_INLINE value_type operator*() const noexcept { return storage_ptr_->composite(id_); }

        [[nodiscard]] uint32_t id() const noexcept { return id_; }

       private:
        const storage_type* storage_ptr_;
        uint32_t id_{0};
      };

      [[nodiscard]] PROMPP_ALWAYS_INLINE auto begin() const noexcept { return iterator_type{*storage_ptr, 0}; }
      [[nodiscard]] PROMPP_ALWAYS_INLINE auto end() const noexcept { return iterator_type{*storage_ptr, storage_ptr->items_.size()}; }

      [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return storage_ptr->count(); }
      [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t next_item_index() const noexcept { return storage_ptr->next_item_index(); }

      [[nodiscard]] PROMPP_ALWAYS_INLINE label_name_sets_table_type::storage_type::view_type label_name_sets() const noexcept {
        return storage_ptr->label_name_sets_table_.data_view();
      }

      [[nodiscard]] PROMPP_ALWAYS_INLINE keys_view_type keys() const noexcept { return storage_ptr->label_name_sets_table_.data_view().symbols(); }
      [[nodiscard]] PROMPP_ALWAYS_INLINE values_view_type values() const noexcept { return label_sets_values_view{.storage_ptr = storage_ptr}; }
      [[nodiscard]] PROMPP_ALWAYS_INLINE values_view_type::values_symbols_view_type values(uint32_t key_id) const noexcept {
        if constexpr (BareBones::concepts::is_dereferenceable<typename symbols_tables_type::value_type>) {
          return (*storage_ptr->symbols_tables_[key_id]).data_view();
        } else {
          return storage_ptr->symbols_tables_[key_id].data_view();
        }
      }

      [[nodiscard]] PROMPP_ALWAYS_INLINE values_view_type::iterator_type::value_type key_symbol(uint32_t key_id) const noexcept { return keys()[key_id]; }
      [[nodiscard]] PROMPP_ALWAYS_INLINE values_view_type::iterator_type::value_type value_symbol(uint32_t key_id, uint32_t value_id) const noexcept {
        return values(key_id)[value_id];
      }
    };

    using view_type = label_set_view;

    storage_type() noexcept = default;
    template <class AnotherStorageType>
      requires kIsReadOnly
    explicit storage_type(const AnotherStorageType& other)
        : symbols_ids_sequences_(other.symbols_ids_sequences_),
          label_name_sets_table_(other.label_name_sets_table_),
          next_item_index_(other.next_item_index_),
          shrinked_size_(other.shrinked_size_),
          items_{other.items_} {
      symbols_tables_.reserve_and_write(other.symbols_tables_.size(), [&other](auto memory, uint32_t size) {
        for (auto& symbol_table : other.symbols_tables_) {
          std::construct_at(memory++, symbol_table);
        }
        return size;
      });
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t count() const noexcept { return static_cast<uint32_t>(items_.size()); }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t remainder_size() const noexcept {
      constexpr uint32_t max_ui32 = std::numeric_limits<uint32_t>::max();

      assert(symbols_ids_sequences_.size() <= max_ui32);

      uint32_t remainder_for_label_sets_table = label_name_sets_table_.remainder_size();
      uint32_t remainder_for_symbols_ids_sequences = max_ui32 - static_cast<uint32_t>(symbols_ids_sequences_.size());
      uint32_t remainder_for_symbol_table = std::numeric_limits<uint32_t>::max();
      for (const auto& table : symbols_tables_) {
        if (const uint32_t n = table.remainder_size(); n < remainder_for_symbol_table) {
          remainder_for_symbol_table = n;
        }
      }
      return std::min({remainder_for_label_sets_table, remainder_for_symbols_ids_sequences, remainder_for_symbol_table});
    }

    template <class AnotherStorageType>
    PROMPP_ALWAYS_INLINE void reserve(const AnotherStorageType& other) noexcept {
      symbols_ids_sequences_.reserve(other.symbols_ids_sequences_.size());
      symbols_tables_.reserve(other.symbols_tables_.size());
      label_name_sets_table_.reserve(other.label_name_sets_table_);
      items_.reserve(other.items_.size());
    }

    PROMPP_ALWAYS_INLINE void reserve(uint32_t count) noexcept { items_.reserve(count); }

    [[nodiscard]] PROMPP_ALWAYS_INLINE composite_type composite(uint32_t id) const noexcept {
      const auto [lns_id, pos] = items_[id];

      auto lns = label_name_sets_table_[lns_id];
      auto values_begin = BareBones::StreamVByte::decoder<symbol_ids_codec_type>(symbols_ids_sequences_.begin() + pos - shrinked_size_, lns.size()).first;

      return composite_type(&symbols_tables_, std::move(lns), std::move(values_begin), lns_id);
    }

    void validate(uint32_t id) const {
      const auto [lns_id, pos] = items_[id];

      if (lns_id >= label_name_sets_table_.size()) {
        throw BareBones::Exception(0x48dd6c9d357d3a7e,
                                   "LabelSets data validation error: expected LabelSets length is out of label name sets table vector range");
      }

      const auto& lns = label_name_sets_table_[lns_id];

      // check that streamvbyte keys are in range
      auto keys_size = BareBones::StreamVByte::keys_size(lns.size());
      if (pos - shrinked_size_ + keys_size > symbols_ids_sequences_.size()) {
        throw BareBones::Exception(0x22f5a82dd120e0e7, "LabelSets data validation error: expected LabelSets keys length is out of data symbols vector range");
      }

      // check that streamvbyte data is in range
      const uint32_t data_size =
          BareBones::StreamVByte::decode_data_size<BareBones::StreamVByte::Codec1234>(lns.size(), symbols_ids_sequences_.begin() + pos - shrinked_size_);
      if (pos - shrinked_size_ + keys_size + data_size > symbols_ids_sequences_.size()) {
        throw BareBones::Exception(0xd02e54dac8e1d328, "LabelSets data validation error: expected LabelSets values length is out of data symbols vector range");
      }

      // check that all symbols are in range
      auto values_begin =
          BareBones::StreamVByte::decoder<BareBones::StreamVByte::Codec1234>(symbols_ids_sequences_.begin() + pos - shrinked_size_, lns.size()).first;
      for (auto i = lns.begin(); i != lns.end(); ++i) {
        if (*values_begin++ >= symbols_tables_[i.id()]->size()) {
          throw BareBones::Exception(0x0f0c520ad6285f15,
                                     "LabelSets data validation error: expected LabelSets symbols length is out of data symbols vector range");
        }
      }
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t allocated_memory() const noexcept {
      return BareBones::mem::allocated_memory(symbols_tables_) + BareBones::mem::allocated_memory(symbols_ids_sequences_) +
             BareBones::mem::allocated_memory(label_name_sets_table_) + BareBones::mem::allocated_memory(items_);
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t next_item_index() const noexcept { return static_cast<uint32_t>(items_.size()); }

    template <class OtherLabelSet, class Cache = NoCache>
    PROMPP_ALWAYS_INLINE uint32_t emplace_back(const OtherLabelSet& label_set, Cache&& cache = {}) noexcept {
      const uint32_t id = items_.size();

      const uint32_t lns_id = find_or_emplace_label_names_set(label_set, std::forward<Cache>(cache));
      const uint32_t pos = symbols_ids_sequences_.size() + shrinked_size_;
      symbols_tables_.resize(label_name_sets_table_.data_view().symbols().size());

      auto lns = label_name_sets_table_[lns_id];
      auto lns_i = lns.begin();
      auto size_before = symbols_ids_sequences_.size();
      auto i = BareBones::StreamVByte::back_inserter<symbol_ids_codec_type>(symbols_ids_sequences_, lns.size());
      for (auto it = label_set.begin(); it != label_set.end(); ++it) {
        *i++ = find_or_emplace_symbol(lns_i.id(), it, std::forward<Cache>(cache));
        ++lns_i;
      }

      next_item_index_ += symbols_ids_sequences_.size() - size_before;

      items_.emplace_back(lns_id, pos);

      return id;
    }

    void shrink() noexcept {
      shrinked_size_ += symbols_ids_sequences_.size();

      items_.resize(0);
      items_.shrink_to_fit();
      symbols_ids_sequences_.resize(0);
      symbols_ids_sequences_.shrink_to_fit();
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE checkpoint_type checkpoint() const noexcept {
      auto checkpoint = checkpoint_type{label_name_sets_table_.checkpoint(),
                                        {},
                                        next_item_index_,
                                        static_cast<uint32_t>(symbols_ids_sequences_.size()),
                                        static_cast<uint32_t>(items_.size())};
      checkpoint.symbols_tables_checkpoints_.reserve_and_write(symbols_tables_.size(), [this](auto memory, uint32_t size) {
        for (auto& symbol_table : this->symbols_tables_) {
          std::construct_at(memory++, symbol_table.checkpoint());
        }
        return size;
      });
      return checkpoint;
    }

    PROMPP_ALWAYS_INLINE void rollback(const checkpoint_type& checkpoint) noexcept {
      assert(checkpoint.size_ <= symbols_ids_sequences_.size());
      symbols_ids_sequences_.resize(checkpoint.size_);

      label_name_sets_table_.rollback(checkpoint.label_name_sets_table_checkpoint_);

      auto symbols_tables_checkpoints = checkpoint.symbols_tables_checkpoints_;
      assert(symbols_tables_checkpoints.size() <= symbols_tables_.size());
      for (uint32_t i = 0; i != symbols_tables_checkpoints.size(); ++i) {
        symbols_tables_[i].rollback(symbols_tables_checkpoints[i]);
      }
      symbols_tables_.resize(symbols_tables_checkpoints.size());
      items_.resize(checkpoint.items_size);
    }

    template <class InputStream>
    auto load(InputStream& in, uint32_t items_size, uint8_t table_version) {
      const auto items_original_size = items_.size();
      if (table_version == 1) {
        // read items
        items_.resize(items_original_size + items_size);
        in.read(reinterpret_cast<char*>(&items_[items_original_size]), sizeof(item_type) * items_size);
      }

      // read version
      const uint8_t version = in.get();
      if (version != 1) {
        throw BareBones::Exception(0x7524f0b0ab963554, "Invalid stream data version (%d) for loading LabelSets into data vector, only version 1 is supported",
                                   version);
      }

      if (table_version == 2) {
        // read items
        items_.resize(items_original_size + items_size);
        in.read(reinterpret_cast<char*>(&items_[items_original_size]), sizeof(item_type) * items_size);
      }

      // read mode
      const auto mode = static_cast<BareBones::SnugComposite::SerializationMode>(in.get());

      // read pos of first seq in the portion, if we are reading wal
      uint32_t first_to_load_i = 0;
      if (mode == BareBones::SnugComposite::SerializationMode::DELTA) {
        in.read(reinterpret_cast<char*>(&first_to_load_i), sizeof(first_to_load_i));
      }
      if (first_to_load_i != next_item_index_) {
        if (mode == BareBones::SnugComposite::SerializationMode::SNAPSHOT) {
          throw BareBones::Exception(0xefdd57cef4b89243, "Attempt to load snapshot into non-empty LabelSets data vector");
        } else if (first_to_load_i < symbols_ids_sequences_.size()) {
          throw BareBones::Exception(0xfead3117c5a549bd, "Attempt to load segment over existing LabelSets data");
        } else {
          throw BareBones::Exception(0xbb996a8ffbcbb53b,
                                     "Attempt to load incomplete data from segment, LabelSets data vector length (%u) is less than segment size (%d)",
                                     symbols_ids_sequences_.size(), first_to_load_i);
        }
      }

      // read size
      uint32_t size_to_load = 0;
      in.read(reinterpret_cast<char*>(&size_to_load), sizeof(size_to_load));

      // read data
      auto sg1 = std::experimental::scope_fail([original_size = symbols_ids_sequences_.size(), this] { symbols_ids_sequences_.resize(original_size); });

      symbols_ids_sequences_.reserve_and_write(size_to_load + sizeof(symbol_ids_codec_type::value_type), [&in, size_to_load](uint8_t* buffer, uint32_t) {
        in.read(reinterpret_cast<char*>(buffer), size_to_load * sizeof(symbols_ids_sequences_[first_to_load_i]));
        return size_to_load;
      });
      next_item_index_ += size_to_load;

      // read label name sets table
      auto label_name_sets_table_checkpoint = label_name_sets_table_.checkpoint();
      auto sg2 = std::experimental::scope_fail([&]() { label_name_sets_table_.rollback(label_name_sets_table_checkpoint); });
      label_name_sets_table_.load(in);

      // read number of tables
      uint32_t number_of_symbols_tables_to_load = 0;
      in.read(reinterpret_cast<char*>(&number_of_symbols_tables_to_load), sizeof(number_of_symbols_tables_to_load));

      // read tables
      auto original_symbols_tables_size = symbols_tables_.size();
      BareBones::Vector<std::pair<uint32_t, typename label_values_symbols_table_type::checkpoint_type>> symbols_tables_checkpoints;
      auto sg3 = std::experimental::scope_fail([&]() {
        for (const auto& [id, checkpoint] : symbols_tables_checkpoints) {
          symbols_tables_[id].rollback(checkpoint);
        }
        symbols_tables_.resize(original_symbols_tables_size);
      });
      for (uint32_t i = 0; i < number_of_symbols_tables_to_load; ++i) {
        // read id
        uint32_t id;

        if (mode == BareBones::SnugComposite::SerializationMode::DELTA) {
          in.read(reinterpret_cast<char*>(&id), sizeof(id));
        } else {
          id = i;
        }

        // resize, if needed
        if (id >= symbols_tables_.size()) [[unlikely]] {
          if (id > symbols_tables_.size()) [[unlikely]] {
            throw BareBones::Exception(0x13fe3e1aae45bb34, "Symbol id sequence is incorrect: id (%u), size: (%u)", id, symbols_tables_.size());
          }

          const auto number_of_tables_stil_left_to_load = number_of_symbols_tables_to_load - i;
          uint64_t size_will_be_at_least = static_cast<uint64_t>(symbols_tables_.size()) + number_of_tables_stil_left_to_load;
          if (size_will_be_at_least >= std::numeric_limits<uint32_t>::max()) [[unlikely]] {
            throw BareBones::Exception(0x98d95ce3b05ec2b5, "Max symbol id (%lu) is greater than UINT32_MAX", size_will_be_at_least);
          }

          symbols_tables_.resize(size_will_be_at_least);
        }

        // read symbols table
        if (mode == BareBones::SnugComposite::SerializationMode::DELTA && id < original_symbols_tables_size)
          symbols_tables_checkpoints.emplace_back(id, symbols_tables_[id].checkpoint());
        symbols_tables_[id].load(in);
      }

      return std::views::iota(items_original_size, items_.size());
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE view_type view() const noexcept { return {.storage_ptr = this}; }

   private:
    template <template <template <class> class> class, template <template <class> class> class, template <class> class>
    friend struct LabelSet;

    symbols_tables_type symbols_tables_;
    symbols_ids_sequences_type symbols_ids_sequences_;
    label_name_sets_table_type label_name_sets_table_;
    uint32_t next_item_index_{};
    uint32_t shrinked_size_{};
    Vector<item_type> items_;

    template <class LabelSet, class Cache>
    PROMPP_ALWAYS_INLINE uint32_t find_or_emplace_label_names_set(LabelSet& label_set, Cache&& cache) {
      if constexpr (use_find_or_emplace_with_cache<Cache, LabelSet, decltype(label_name_sets_table_), decltype(label_set.names())>) {
        return label_name_sets_table_.find_or_emplace_with_cache(label_set.names(), label_set.id(), cache.name_sets, cache);
      }

      return label_name_sets_table_.find_or_emplace(label_set.names());
    }

    template <class LabelIterator, class Cache>
    PROMPP_ALWAYS_INLINE uint32_t find_or_emplace_symbol(uint32_t lns_id, const LabelIterator& label, Cache&& cache) {
      if constexpr (use_find_or_emplace_with_cache<Cache, LabelIterator, decltype(symbols_tables_[0]), decltype((*label).second)>) {
        const auto name_id = label.name_id();
        return symbols_tables_[lns_id].find_or_emplace_with_cache((*label).second, label.value_id(), cache.values[name_id]);
      }

      return symbols_tables_[lns_id].find_or_emplace((*label).second);
    }
  };
};

}  // namespace PromPP::Primitives::SnugComposites::Filaments
