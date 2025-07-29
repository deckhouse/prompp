#pragma once

#include "common.h"
#include "series_data/concepts.h"
#include "series_data/data_storage.h"

namespace series_data::unloading {
class LoadReverter {
 public:
  explicit LoadReverter(DataStorage& storage) : storage_(storage) {}

  template <LsIDStorageInterface LsIDStorage>
  void set_source_sizes(const LsIDStorage& ls_id_range, uint32_t ls_id_range_count) noexcept {
    source_sizes_.clear();
    source_sizes_.reserve(ls_id_range_count);

    for (uint32_t ls_id : ls_id_range) {
      if (storage_.outdated_chunks.find(ls_id) == storage_.outdated_chunks.end()) {
        for (uint8_t chunk_id = 0; const auto& data : storage_.chunks(ls_id)) {
          if (const auto& chunk = data.chunk(); is_unloadable_encoder(chunk.encoding_state.encoding_type)) {
            source_sizes_.emplace_back(ls_id, get_chunk_stream(storage_, chunk, data.is_open()).size_in_bits(), chunk_id);
          }
          ++chunk_id;
        }
      }
    }
  }

  void revert() noexcept {
    for (const auto& meta : source_sizes_) {
      revert_chunk(meta);
    }
  }

 private:
  struct LsIdSizeChunkId {
    uint32_t ls_id;
    uint16_t source_size_in_bits;
    uint8_t chunk_id;
  };

  void revert_chunk(const LsIdSizeChunkId& meta) const noexcept {
    encoder::CompactBitSequence seq;

    const auto& chunk_data = std::ranges::next(DataStorage::SeriesChunkIterator{&storage_, meta.ls_id}, meta.chunk_id, DataStorage::SeriesChunks::end());

    auto& chunk_bit_sequence = get_chunk_stream(storage_, chunk_data->chunk(), chunk_data->is_open());

    seq.push_back_bytes(chunk_bit_sequence.raw_bytes() + BareBones::Bit::to_ceil_bytes(chunk_bit_sequence.size_in_bits() - meta.source_size_in_bits),
                        meta.source_size_in_bits);

    chunk_bit_sequence = std::move(seq);
  }

  BareBones::Vector<LsIdSizeChunkId> source_sizes_;
  DataStorage& storage_;
};
}  // namespace series_data::unloading