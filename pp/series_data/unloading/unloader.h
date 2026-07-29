#pragma once

#include "common.h"
#include "snapshot.h"

#include "bare_bones/bit.h"
#include "bare_bones/preprocess.h"
#include "series_data/data_storage.h"
#include "series_data/encoder/bit_sequence.h"

namespace series_data::unloading {

class Unloader {
 public:
  explicit Unloader(DataStorage& storage) : storage_(storage) {}

  template <class Stream>
  void create_snapshot(Stream& stream) {
    unloaded_chunks_.clear();
    select_chunks_to_unload();
    SnapshotWriter::write_to(stream, SnapshotChunkRange{storage_, unloaded_chunks_});
  }

  void unload() {
    for (const auto chunk : unloaded_chunks_) {
      if (!storage_.queried_series_bitmap.is_set(chunk.ls_id)) {
        get_chunk_stream(storage_, chunk.ls_id, chunk.chunk_id).trim_lower_bytes(chunk.trim_bytes);
        storage_.unloaded_series_bitmap.set(chunk.ls_id);
      }
    }

    unloaded_chunks_.clear();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE DataStorage& storage() noexcept { return storage_; }

 private:
  struct ChunkSize {
    uint32_t ls_id;
    uint16_t trim_bytes;
    uint8_t chunk_id;
  };

  class SnapshotChunkRange {
   public:
    class Iterator {
     public:
      Iterator(DataStorage& storage, const ChunkSize* chunk) noexcept : storage_(storage), chunk_(chunk) {}

      [[nodiscard]] SnapshotChunkView operator*() const noexcept {
        const auto& bitseq = get_chunk_stream(storage_, chunk_->ls_id, chunk_->chunk_id);
        return {chunk_->ls_id, chunk_->chunk_id, {bitseq.raw_bytes(), chunk_->trim_bytes}};
      }

      Iterator& operator++() noexcept {
        ++chunk_;
        return *this;
      }

      [[nodiscard]] bool operator==(const Iterator& other) const noexcept { return chunk_ == other.chunk_; }

     private:
      DataStorage& storage_;
      const ChunkSize* chunk_;
    };

    SnapshotChunkRange(DataStorage& storage, const BareBones::Vector<ChunkSize>& chunks) noexcept : storage_(storage), chunks_(chunks) {}

    [[nodiscard]] Iterator begin() const noexcept { return {storage_, chunks_.data()}; }
    [[nodiscard]] Iterator end() const noexcept { return {storage_, chunks_.data() + chunks_.size()}; }

   private:
    DataStorage& storage_;
    const BareBones::Vector<ChunkSize>& chunks_;
  };

  DataStorage& storage_;
  BareBones::Vector<ChunkSize> unloaded_chunks_;

  void select_chunks_to_unload() noexcept {
    for (uint32_t ls_id = 0; ls_id < storage_.open_chunks.size(); ++ls_id) {
      if (storage_.queried_series_bitmap.is_set(ls_id)) {
        continue;
      }

      const auto& open_chunk = storage_.open_chunks[ls_id];
      if (!is_unloadable_encoder(open_chunk.encoding_state.encoding_type)) {
        continue;
      }

      const auto& bitseq = get_chunk_stream<chunk::DataChunk::Type::kOpen>(storage_, open_chunk);
      if (bitseq.size_in_bits() < BareBones::Bit::kByteBits) {
        continue;
      }

      unloaded_chunks_.emplace_back(ChunkSize{
          .ls_id = ls_id,
          .trim_bytes = static_cast<uint16_t>(BareBones::Bit::to_bytes(bitseq.size_in_bits())),
          .chunk_id = static_cast<uint8_t>(get_open_chunk_id(ls_id)),
      });
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t get_open_chunk_id(uint32_t ls_id) const noexcept {
    if (const auto it = storage_.finalized_chunks.find(ls_id); it != storage_.finalized_chunks.end()) {
      return it->second.count();
    }
    return 0;
  }
};
}  // namespace series_data::unloading
