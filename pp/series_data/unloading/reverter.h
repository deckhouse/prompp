#pragma once

#include "common.h"
#include "series_data/concepts.h"
#include "series_data/data_storage.h"

namespace series_data::unloading {
class LoadReverter {
 public:
  explicit LoadReverter(DataStorage& storage) : storage_(storage) {}

  template <LsIDStorageInterface LsIDStorage>
  void add_series_to_revert(const LsIDStorage& ls_id_range, uint32_t ls_id_range_count) noexcept {
    add_series_to_revert(ls_id_range.begin(), ls_id_range.end(), ls_id_range_count);
  }

  template <class LsIdSetIterator, class LsIdSetIteratorSentinel>
  void add_series_to_revert(LsIdSetIterator ls_id_iterator, LsIdSetIteratorSentinel ls_id_end_iterator, uint32_t count) noexcept {
    source_sizes_.reserve(source_sizes_.size() + count);

    for (; ls_id_iterator != ls_id_end_iterator; ++ls_id_iterator) {
      if (storage_.outdated_chunks.find(*ls_id_iterator) == storage_.outdated_chunks.end()) {
        process_series(*ls_id_iterator);
      }
    }
  }

  void revert() noexcept {
    for (const auto& meta : source_sizes_) {
      if (!storage_.queried_series_bitmap.is_set(meta.ls_id)) {
        revert_chunk(meta);
      }
    }

    source_sizes_.clear();
  }

 private:
  struct LsIdSizeChunkId {
    uint32_t ls_id;
    uint16_t source_size_in_bits;
    uint8_t chunk_id;
  };

  PROMPP_ALWAYS_INLINE void process_series(uint32_t ls_id) noexcept {
    for (uint8_t chunk_id = 0; const auto& data : storage_.chunks(ls_id)) {
      if (const auto& chunk = data.chunk(); is_unloadable_encoder(chunk.encoding_state.encoding_type)) {
        source_sizes_.emplace_back(ls_id, get_chunk_stream(storage_, chunk, data.is_open()).size_in_bits(), chunk_id);
      }
      ++chunk_id;
    }
  }

  PROMPP_ALWAYS_INLINE void revert_chunk(const LsIdSizeChunkId& meta) const noexcept {
    const auto& chunk_data = std::ranges::next(DataStorage::SeriesChunkIterator{&storage_, meta.ls_id}, meta.chunk_id, DataStorage::SeriesChunks::end());
    auto& chunk_bit_sequence = get_chunk_stream(storage_, chunk_data->chunk(), chunk_data->is_open());

    if (meta.source_size_in_bits == chunk_bit_sequence.size_in_bits()) {
      return;
    }

    encoder::CompactBitSequence seq;
    seq.push_back_bytes(chunk_bit_sequence.raw_bytes() + BareBones::Bit::to_ceil_bytes(chunk_bit_sequence.size_in_bits() - meta.source_size_in_bits),
                        meta.source_size_in_bits);
    chunk_bit_sequence = std::move(seq);

    storage_.unloaded_series_bitmap.set(meta.ls_id);
  }

  BareBones::Vector<LsIdSizeChunkId> source_sizes_;
  DataStorage& storage_;
};
}  // namespace series_data::unloading