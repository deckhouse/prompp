#include <limits>
#include <sstream>

#include <gtest/gtest.h>

#include "bare_bones/snug_composite.h"

namespace {

using BareBones::Vector;
using std::string_literals::operator""s;

template <template <class> class Vector>
struct TestStringFilament {
  struct storage_type {
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
          out.write(reinterpret_cast<const char*>(&storage.items[id_offset]), sizeof(item_type) * id_count);

          // write a version
          out.put(1);
        } else {  // table_version == 2
          // write a version
          out.put(1);

          // write items
          out.write(reinterpret_cast<const char*>(&storage.items[id_offset]), sizeof(item_type) * id_count);
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
          out.write(&storage.data[first_to_save], size_to_save);
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

        [[nodiscard]] PROMPP_ALWAYS_INLINE value_type operator*() const noexcept { return storage_ptr_->composite(id_); }

        [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t id() const noexcept { return id_; }

       private:
        const storage_type* storage_ptr_;
        uint32_t id_{0};
      };

      [[nodiscard]] PROMPP_ALWAYS_INLINE auto begin() const noexcept { return iterator_type{*storage_ptr, 0}; }
      [[nodiscard]] PROMPP_ALWAYS_INLINE auto end() const noexcept { return iterator_type{*storage_ptr, storage_ptr->items.size()}; }

      [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t size() const noexcept { return storage_ptr->count(); }
      [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t next_item_index() const noexcept { return storage_ptr->count(); }
    };

    using view_type = symbols_view;

    storage_type() noexcept = default;
    template <class AnotherStorageType>
      requires kIsReadOnly
    explicit storage_type(const AnotherStorageType& other) : data{other.data}, items{other.items} {}

    Vector<char> data;
    Vector<item_type> items;

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t count() const noexcept { return static_cast<uint32_t>(items.size()); }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t remainder_size() const noexcept {
      constexpr uint32_t max_ui32 = std::numeric_limits<uint32_t>::max();
      assert(data.size() <= max_ui32);
      return max_ui32 - static_cast<uint32_t>(data.size());
    }

    template <class AnotherStorageType>
    PROMPP_ALWAYS_INLINE void reserve(const AnotherStorageType& other) noexcept {
      items.reserve(other.items.size());
      data.reserve(other.data.size());
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE composite_type composite(uint32_t id) const noexcept {
      const auto item = items[id];
      return std::string_view(data.data() + item.pos, item.length);
    }

    void validate(uint32_t id) const {
      const auto item = items[id];
      if (item.pos + item.length > data.size()) {
        throw BareBones::Exception(0x75555f55ebe357a3, "TestStringFilament validation error: length is out of data vector range");
      }
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t allocated_memory() const noexcept {
      return BareBones::mem::allocated_memory(data) + BareBones::mem::allocated_memory(items);
    }

    PROMPP_ALWAYS_INLINE uint32_t emplace_back(composite_type str) noexcept {
      const auto id = static_cast<uint32_t>(items.size());
      const auto pos = static_cast<uint32_t>(data.size());
      items.emplace_back(pos, str.length());
      data.push_back(str.begin(), str.end());
      return id;
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE checkpoint_type checkpoint() const noexcept {
      return {static_cast<uint32_t>(data.size()), static_cast<uint32_t>(items.size())};
    }
    PROMPP_ALWAYS_INLINE void rollback(const checkpoint_type& checkpoint) noexcept {
      assert(checkpoint.data_size <= data.size());
      assert(checkpoint.items_size <= items.size());
      data.resize(checkpoint.data_size);
      items.resize(checkpoint.items_size);
    }

    template <class InputStream>
    auto load(InputStream& in, uint32_t items_size, uint8_t table_version) {
      const auto original_size = items.size();
      if (table_version == 1) {
        // read items
        items.resize(original_size + items_size);
        in.read(reinterpret_cast<char*>(&items[original_size]), sizeof(item_type) * items_size);
      }

      // read version
      const uint8_t version = in.get();
      if (version != 1) {
        throw BareBones::Exception(
            0x67c010edbd64e272, "Invalid stream data version (%d) for loading into data vector (TestStringFilament::storage_type), only version 1 is supported",
            version);
      }

      if (table_version == 2) {
        // read items
        items.resize(original_size + items_size);
        in.read(reinterpret_cast<char*>(&items[original_size]), sizeof(item_type) * items_size);
      }

      // read mode
      const auto mode = static_cast<BareBones::SnugComposite::SerializationMode>(in.get());

      // read pos of the first symbol in the portion if we are reading wal
      uint32_t first_to_load_i = 0;
      if (mode == BareBones::SnugComposite::SerializationMode::DELTA) {
        in.read(reinterpret_cast<char*>(&first_to_load_i), sizeof(first_to_load_i));
      }

      if (first_to_load_i != data.size()) {
        if (mode == BareBones::SnugComposite::SerializationMode::SNAPSHOT) {
          throw BareBones::Exception(0x4c0ca0586da6da3f, "Attempt to load snapshot into non-empty data vector");
        } else if (first_to_load_i < data.size()) {
          throw BareBones::Exception(0x55cb9b02c23f7bbd, "Attempt to load segment over existing data");
        } else {
          throw BareBones::Exception(0x55cb9b02c23f7bbd, "Attempt to load incomplete data from segment, data vector length (%u) is less than segment size (%d)",
                                     data.size(), first_to_load_i);
        }
      }

      // read size
      uint32_t size_to_load = 0;
      in.read(reinterpret_cast<char*>(&size_to_load), sizeof(size_to_load));

      // read data
      data.resize(data.size() + size_to_load);
      in.read(data.begin() + first_to_load_i, size_to_load);

      return std::views::iota(original_size, items.size());
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE view_type view() const noexcept { return {.storage_ptr = this}; }

    void drop_front(uint32_t drop_count) {
      if (drop_count != count()) [[unlikely]] {
        throw BareBones::Exception(0x1bf0dbff9fe3d955, "Unsupported drop for tests");
      }
      items.clear();
    }
  };
};

using EncodingBimap = BareBones::SnugComposite::EncodingBimap<TestStringFilament, Vector>;
using ShrinkableEncodingBimap = BareBones::SnugComposite::ShrinkableEncodingBimap<TestStringFilament, Vector>;

class EncodingBimapFixture : public testing::Test {
 protected:
  EncodingBimap table_;

  static constexpr auto kInvalidId = std::numeric_limits<uint32_t>::max();
};

TEST_F(EncodingBimapFixture, Empty) {
  // Arrange

  // Act

  // Assert
  EXPECT_EQ(0U, table_.items_count());
}

TEST_F(EncodingBimapFixture, FindOrEmplaceNewValue) {
  // Arrange
  const auto value = "test"s;

  // Act
  const auto expected_id = table_.next_item_index();
  const auto id = table_.find_or_emplace(value);

  // Assert
  EXPECT_EQ(expected_id, id);
  EXPECT_EQ(1U, table_.items_count());
}

TEST_F(EncodingBimapFixture, FindOrEmplaceExistingValue) {
  // Arrange
  const auto value = "test"s;
  const auto id1 = table_.find_or_emplace(value);

  // Act
  const auto id2 = table_.find_or_emplace(value);

  // Assert
  EXPECT_EQ(id1, id2);
  EXPECT_EQ(1U, table_.items_count());
}

TEST_F(EncodingBimapFixture, FindOrEmplaceDifferentValues) {
  // Arrange
  const std::string value1 = "test1"s;
  const std::string value2 = "test2"s;

  // Act
  const auto id1 = table_.find_or_emplace(value1);
  const auto id2 = table_.find_or_emplace(value2);

  // Assert
  EXPECT_NE(id1, id2);
  EXPECT_EQ(2U, table_.items_count());
}

TEST_F(EncodingBimapFixture, FindOrEmplaceWithHash) {
  // Arrange
  const std::string value = "test"s;
  const auto hash_val = BareBones::XXHash3::hash(value);

  // Act
  const auto id1 = table_.find_or_emplace(value, hash_val);
  const auto id2 = table_.find_or_emplace(value, hash_val);

  // Assert
  EXPECT_EQ(id1, id2);
  EXPECT_EQ(1U, table_.items_count());
}

TEST_F(EncodingBimapFixture, FindExistingValue) {
  // Arrange
  table_.find_or_emplace("test1"s);
  table_.find_or_emplace("test2"s);
  const std::string value = "test"s;
  const auto id = table_.find_or_emplace(value);

  // Act
  const auto found_id = table_.find(value);

  // Assert
  ASSERT_TRUE(found_id.has_value());
  EXPECT_EQ(id, found_id.value());
}

TEST_F(EncodingBimapFixture, FindNonExistingValue) {
  // Arrange
  table_.find_or_emplace("test1"s);
  table_.find_or_emplace("test2"s);
  const std::string value = "test"s;

  // Act
  const auto found_id = table_.find(value);

  // Assert
  EXPECT_FALSE(found_id.has_value());
  EXPECT_EQ(kInvalidId, table_.find(value).value_or(kInvalidId));
}

TEST_F(EncodingBimapFixture, FindWithHash) {
  // Arrange
  const std::string value = "test"s;
  const auto hash_val = BareBones::XXHash3::hash(value);
  const auto id = table_.find_or_emplace(value, hash_val);

  // Act
  const auto found_id = table_.find(value, hash_val);

  // Assert
  ASSERT_TRUE(found_id.has_value());
  EXPECT_EQ(id, found_id.value());
}

TEST_F(EncodingBimapFixture, OperatorBracket) {
  // Arrange
  table_.find_or_emplace("test1"s);
  table_.find_or_emplace("test2"s);
  const std::string value = "test"s;
  const auto id = table_.find_or_emplace(value);

  // Act
  const auto composite = table_[id];

  // Assert
  EXPECT_EQ(value, composite);
}

TEST_F(EncodingBimapFixture, Checkpoint) {
  // Arrange
  table_.find_or_emplace("test1"s);
  table_.find_or_emplace("test2"s);

  // Act
  const auto checkpoint = table_.checkpoint();

  // Assert
  EXPECT_EQ(2U, checkpoint.size());
}

TEST_F(EncodingBimapFixture, Rollback) {
  // Arrange
  table_.find_or_emplace("test1"s);
  const auto checkpoint = table_.checkpoint();
  table_.find_or_emplace("test2"s);

  // Act
  table_.rollback(checkpoint);

  // Assert
  EXPECT_EQ(1U, table_.items_count());
  EXPECT_TRUE(table_.find("test1"s).has_value());
  EXPECT_FALSE(table_.find("test2"s).has_value());
}

TEST_F(EncodingBimapFixture, RollbackToEmpty) {
  // Arrange
  const auto checkpoint = table_.checkpoint();
  table_.find_or_emplace("test1"s);

  // Act
  table_.rollback(checkpoint);

  // Assert
  EXPECT_EQ(0U, table_.items_count());
}

TEST_F(EncodingBimapFixture, SerializeDeserializeSnapshot) {
  // Arrange
  table_.find_or_emplace("test1"s);
  table_.find_or_emplace("test2"s);
  std::stringstream stream;

  // Act
  stream << table_;
  EncodingBimap table2;
  stream >> table2;

  // Assert
  EXPECT_EQ(table_.items_count(), table2.items_count());
  EXPECT_EQ("test1"s, table2[0]);
  EXPECT_EQ("test2"s, table2[1]);
}

TEST_F(EncodingBimapFixture, SerializeDeserializeDelta) {
  // Arrange
  table_.find_or_emplace("test1"s);
  const auto checkpoint1 = table_.checkpoint();
  table_.find_or_emplace("test2"s);
  const auto checkpoint2 = table_.checkpoint();
  std::stringstream stream;

  // Act
  table_.save(stream, checkpoint2 - checkpoint1);
  EncodingBimap table2;
  table2.find_or_emplace("test1"s);
  stream >> table2;

  // Assert
  EXPECT_EQ(2U, table2.items_count());
  EXPECT_EQ("test1"s, table2[0]);
  EXPECT_EQ("test2"s, table2[1]);
}

TEST_F(EncodingBimapFixture, DataView) {
  // Arrange
  table_.find_or_emplace("test1"s);
  table_.find_or_emplace("test2"s);

  // Act
  const auto view = table_.data_view();

  // Assert
  EXPECT_EQ(2U, view.size());
  EXPECT_EQ("test1"s, *view.begin());
  EXPECT_EQ("test2"s, *std::next(view.begin()));
}

class ShrinkableEncodingBimapFixture : public testing::Test {
 protected:
  ShrinkableEncodingBimap table_;
  static constexpr auto kInvalidId = std::numeric_limits<uint32_t>::max();
};

TEST_F(ShrinkableEncodingBimapFixture, Empty) {
  // Arrange

  // Act

  // Assert
  EXPECT_EQ(0U, table_.items_count());
}

TEST_F(ShrinkableEncodingBimapFixture, FindOrEmplace) {
  // Arrange
  const std::string value = "test"s;

  // Act
  const auto id = table_.find_or_emplace(value);

  // Assert
  EXPECT_EQ(0U, id);
  EXPECT_EQ(1U, table_.items_count());
}

TEST_F(ShrinkableEncodingBimapFixture, FindOrEmplaceExisting) {
  // Arrange
  const std::string value = "test"s;
  const auto id1 = table_.find_or_emplace(value);

  // Act
  const auto id2 = table_.find_or_emplace(value);

  // Assert
  EXPECT_EQ(id1, id2);
  EXPECT_EQ(1U, table_.items_count());
}

TEST_F(ShrinkableEncodingBimapFixture, Checkpoint) {
  // Arrange
  table_.find_or_emplace("test"s);

  // Act
  const auto checkpoint = table_.checkpoint();

  // Assert
  EXPECT_EQ(1U, checkpoint.size());
}

TEST_F(ShrinkableEncodingBimapFixture, ShrinkToCheckpointSize) {
  // Arrange
  table_.find_or_emplace("test1"s);
  const auto checkpoint = table_.checkpoint();

  // Act
  table_.shrink_to_checkpoint_size(checkpoint);

  // Assert
  EXPECT_EQ(0U, table_.items_count());
}

TEST_F(ShrinkableEncodingBimapFixture, EmplaceAfterShrink) {
  // Arrange
  const auto id_before = table_.find_or_emplace("test1"s);
  table_.shrink_to_checkpoint_size(table_.checkpoint());

  // Act
  const auto id_after = table_.find_or_emplace("test1"s);

  // Assert
  EXPECT_EQ(0U, id_before);
  EXPECT_EQ(1U, id_after);
  EXPECT_EQ(1U, table_.items_count());

  EXPECT_EQ("test1"s, table_[id_after]);
}

TEST_F(ShrinkableEncodingBimapFixture, ShrinkToOutdatedCheckpointThrows) {
  // Arrange
  table_.find_or_emplace("test1"s);

  const auto checkpoint = table_.checkpoint();
  table_.shrink_to_checkpoint_size(checkpoint);

  table_.find_or_emplace("test2"s);

  // Act

  // Assert
  EXPECT_THROW(table_.shrink_to_checkpoint_size(checkpoint), BareBones::Exception);
}

TEST_F(ShrinkableEncodingBimapFixture, OperatorBracketWithShift) {
  // Arrange
  table_.find_or_emplace("test1"s);
  table_.shrink_to_checkpoint_size(table_.checkpoint());
  const auto id = table_.find_or_emplace("test2"s);

  // Act
  const auto composite = table_[id];

  // Assert
  EXPECT_EQ("test2"s, composite);
}

TEST_F(ShrinkableEncodingBimapFixture, SerializeDeserializeSnapshot) {
  // Arrange
  table_.find_or_emplace("test1"s);
  table_.find_or_emplace("test2"s);
  const auto checkpoint = table_.checkpoint();
  std::stringstream stream;

  // Act
  table_.save(stream, checkpoint);
  ShrinkableEncodingBimap table2;
  stream >> table2;

  // Assert
  EXPECT_EQ(table_.items_count(), table2.items_count());
  EXPECT_EQ("test1"s, table2[0]);
  EXPECT_EQ("test2"s, table2[1]);
}

TEST_F(ShrinkableEncodingBimapFixture, SerializeDeserializeDeltaAfterShrink) {
  // Arrange
  ShrinkableEncodingBimap table2;
  std::stringstream snapshot_stream;

  table_.find_or_emplace("test1"s);
  table_.find_or_emplace("test2"s);
  const auto initial_checkpoint = table_.checkpoint();

  table_.save(snapshot_stream, initial_checkpoint);
  snapshot_stream >> table2;
  table2.shrink_to_checkpoint_size(table2.checkpoint());

  const auto test3_id = table_.find_or_emplace("test3"s);
  const auto test4_id = table_.find_or_emplace("test4"s);
  const auto final_checkpoint = table_.checkpoint();
  const auto delta = final_checkpoint - initial_checkpoint;

  // Act
  std::stringstream delta_stream;
  table_.save(delta_stream, delta);
  delta_stream >> table2;

  // Assert
  EXPECT_EQ(2U, table2.items_count());

  EXPECT_EQ(kInvalidId, table2.find("test1"s).value_or(kInvalidId));
  EXPECT_EQ(kInvalidId, table2.find("test2"s).value_or(kInvalidId));
  EXPECT_EQ(test3_id, table2.find("test3"s).value_or(kInvalidId));
  EXPECT_EQ(test4_id, table2.find("test4"s).value_or(kInvalidId));
}

}  // namespace
