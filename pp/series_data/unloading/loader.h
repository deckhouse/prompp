#pragma once

#include <utility>

#include "common.h"

#include "bare_bones/bitset.h"
#include "bare_bones/encoding.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/type_traits.h"
#include "series_data/concepts.h"
#include "series_data/data_storage.h"
#include "series_data/encoder.h"
#include "series_data/outdated_chunk_merger.h"

namespace series_data::unloading {
struct PROMPP_ATTRIBUTE_PACKED SeriesToLoadInfo {
  uint8_t chunk_id = 0;
  encoder::CompactBitSequence buffer{};
};
}  // namespace series_data::unloading

template <>
struct BareBones::IsTriviallyReallocatable<series_data::unloading::SeriesToLoadInfo> : std::true_type {};

namespace series_data::unloading {
class Loader {
  class UnorderedVector {
   public:
    [[nodiscard]] PROMPP_ALWAYS_INLINE bool empty() const noexcept { return ls_id_to_offset_.empty(); }

    [[nodiscard]] PROMPP_ALWAYS_INLINE size_t size() const noexcept { return ls_id_to_offset_.size(); }

    PROMPP_ALWAYS_INLINE void reserve(size_t size) noexcept {
      series_to_load_infos_.resize(size);
      ls_id_to_offset_.reserve(size);
    }

    PROMPP_ALWAYS_INLINE void clear() noexcept {
      ls_id_to_offset_.erase(ls_id_to_offset_.begin(), ls_id_to_offset_.end());
      for (auto& info : series_to_load_infos_) {
        info.chunk_id = 0;
        info.buffer.rewind();
      }
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE SeriesToLoadInfo& operator[](uint32_t key) noexcept {
      if (auto it = ls_id_to_offset_.find(key); it != ls_id_to_offset_.end()) [[likely]] {
        return series_to_load_infos_[it->second];
      }

      ls_id_to_offset_[key] = ls_id_to_offset_.size();

      if (ls_id_to_offset_.size() == series_to_load_infos_.size()) [[unlikely]] {
        return series_to_load_infos_.emplace_back();
      }
      return series_to_load_infos_.back();
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE const SeriesToLoadInfo& operator[](uint32_t key) const noexcept {
      assert(ls_id_to_offset_.find(key) != ls_id_to_offset_.end());
      return series_to_load_infos_[ls_id_to_offset_.find(key)->second];
    }

    class IteratorSentinel {};
    template <bool IsConst>
    class Iterator {
      using MapIterator = phmap::flat_hash_map<uint32_t, uint32_t>::const_iterator;
      using VectorPtr = std::conditional_t<IsConst, const UnorderedVector*, UnorderedVector*>;
      using RefType = std::conditional_t<IsConst, const SeriesToLoadInfo&, SeriesToLoadInfo&>;
      using PairType = std::pair<int, RefType>;

     public:
      using iterator_category = std::input_iterator_tag;
      using value_type = PairType;
      using difference_type = std::ptrdiff_t;

      PROMPP_ALWAYS_INLINE Iterator() noexcept = default;
      PROMPP_ALWAYS_INLINE Iterator(MapIterator map_it, VectorPtr parent) noexcept : map_it_(map_it), parent_(parent) {}

      PROMPP_ALWAYS_INLINE PairType operator*() const noexcept { return {map_it_->first, parent_->series_to_load_infos_[map_it_->second]}; }

      PROMPP_ALWAYS_INLINE Iterator& operator++() noexcept {
        ++map_it_;
        return *this;
      }

      PROMPP_ALWAYS_INLINE Iterator operator++(int) noexcept {
        Iterator retval = *this;
        ++map_it_;
        return retval;
      }

      PROMPP_ALWAYS_INLINE bool operator==(const Iterator& other) const { return map_it_ == other.map_it_; }
      PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const { return map_it_ == parent_->ls_id_to_offset_.end(); }

     private:
      MapIterator map_it_{};
      VectorPtr parent_{nullptr};
    };
    using const_iterator = Iterator<true>;
    using iterator = Iterator<false>;
    using sentinel = IteratorSentinel;

    [[nodiscard]] PROMPP_ALWAYS_INLINE iterator begin() noexcept { return {ls_id_to_offset_.begin(), this}; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE const_iterator begin() const noexcept { return {ls_id_to_offset_.begin(), this}; }

    [[nodiscard]] static PROMPP_ALWAYS_INLINE sentinel end() noexcept { return {}; }

    PROMPP_ALWAYS_INLINE iterator find(uint32_t key) noexcept {
      if (auto it = ls_id_to_offset_.find(key); it != ls_id_to_offset_.end()) [[likely]] {
        return {it, this};
      }
      return {ls_id_to_offset_.end(), this};
    }
    [[nodiscard]] PROMPP_ALWAYS_INLINE const_iterator find(uint32_t key) const noexcept {
      if (auto it = ls_id_to_offset_.find(key); it != ls_id_to_offset_.end()) [[likely]] {
        return {it, this};
      }
      return {ls_id_to_offset_.end(), this};
    }

    PROMPP_ALWAYS_INLINE iterator insert(uint32_t key) noexcept {
      if (const auto it = find(key); it != end()) [[unlikely]] {
        auto& [chunk_id, buffer] = (*it).second;
        chunk_id = 0;
        buffer.rewind();
        return it;
      }

      auto map_it = ls_id_to_offset_.insert({key, ls_id_to_offset_.size()});
      if (ls_id_to_offset_.size() == series_to_load_infos_.size()) [[unlikely]] {
        series_to_load_infos_.emplace_back();
      }

      return {map_it.first, this};
    }

   private:
    BareBones::Vector<SeriesToLoadInfo> series_to_load_infos_{};
    phmap::flat_hash_map<uint32_t, uint32_t> ls_id_to_offset_{};
  };

 public:
  explicit Loader(DataStorage& storage) : storage_(storage) {
    ls_id_to_infos_.reserve(storage_.unloaded_series_bitmap.popcount());

    for (uint32_t ls_id : storage_.unloaded_series_bitmap) {
      ls_id_to_infos_.insert(ls_id);
    }

    storage_.unloaded_series_bitmap.clear();
  }

  template <LsIDStorageInterface LsIDStorage>
  explicit Loader(DataStorage& storage, const LsIDStorage& ls_id_range, uint32_t ls_id_range_count) : storage_(storage) {
    ls_id_to_infos_.reserve(ls_id_range_count);

    for (uint32_t ls_id : ls_id_range) {
      ls_id_to_infos_.insert(ls_id);
      storage_.unloaded_series_bitmap.reset(ls_id);
    }
  }

  void load_next(std::span<const uint8_t> buffer) {
    const auto bitset_it = BareBones::Bitset::create_read_iterator(buffer);
    const auto length_it = EncodingChunkLengthSequence::create_read_iterator(buffer, length_encoder_);
    const auto id_it = EncodingChunkIDSequence::create_read_iterator(buffer, id_encoder_);

    const uint8_t* bitseqs_ptr = buffer.data();

    process_ls_id_data(bitset_it, length_it, id_it, bitseqs_ptr);
  }

  template <EncoderInterface Encoder = series_data::Encoder<>>
  void load_finalize() {
    for (const auto& [ls_id, info] : ls_id_to_infos_) {
      if (info.buffer.size_in_bits() != 0) {
        load_chunk_id(ls_id, info);
      }
    }

    Encoder encoder{storage_};
    OutdatedChunkMerger<Encoder> outdated_chunk_merger{encoder};
    for (const auto& ls_id : std::views::keys(ls_id_to_infos_)) {
      outdated_chunk_merger.merge(ls_id);
    }
  }

  [[nodiscard]] bool empty() const noexcept { return ls_id_to_infos_.empty(); }

 private:
  void process_ls_id_data(BareBones::Bitset::Iterator bitset_it,
                          EncodingChunkLengthSequence::Iterator length_it,
                          EncodingChunkIDSequence::Iterator id_it,
                          const uint8_t* bitseqs_ptr) {
    uint32_t accumulated_offset = 0;

    while (bitset_it != BareBones::Bitset::IteratorSentinel{}) {
      const uint32_t ls_id = *bitset_it;

      if (auto infos_it = ls_id_to_infos_.find(ls_id); infos_it != ls_id_to_infos_.end()) {
        auto& info = (*infos_it).second;

        const uint32_t chunk_id_snapshot = *id_it;
        const uint32_t bitseq_size = *length_it;

        if (chunk_id_snapshot != info.chunk_id) {
          if (info.buffer.size_in_bits() != 0) {
            load_chunk_id(ls_id, info);
          }
          info.chunk_id = chunk_id_snapshot;
          info.buffer.rewind();
        }
        info.buffer.push_back_bytes(bitseqs_ptr + accumulated_offset, BareBones::Bit::to_bits(bitseq_size));
      }

      accumulated_offset += *length_it;

      ++bitset_it;
      ++length_it;
      ++id_it;
    }
  }

  void load_chunk_id(uint32_t ls_id, SeriesToLoadInfo& info) const {
    const auto& chunk_data = std::ranges::next(DataStorage::SeriesChunkIterator{&storage_, ls_id}, info.chunk_id, DataStorage::SeriesChunks::end());

    auto& chunk_bit_sequence = [&]() -> encoder::CompactBitSequence& {
      if (chunk_data->is_open()) {
        return get_open_chunk_stream(storage_, ls_id);
      }
      return storage_.finalized_data_streams[chunk_data->chunk().encoder.external_index];
    }();

    info.buffer.push_back_bytes(chunk_bit_sequence.raw_bytes(), chunk_bit_sequence.size_in_bits());

    chunk_bit_sequence = std::move(info.buffer);
  }

  DataStorage& storage_;

  EncodingChunkLengthSequence::encoder_type length_encoder_{};
  EncodingChunkIDSequence::encoder_type id_encoder_{};

  UnorderedVector ls_id_to_infos_{};
};
}  // namespace series_data::unloading