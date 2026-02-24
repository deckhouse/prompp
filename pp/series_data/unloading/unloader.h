#pragma once

#include "common.h"

#include "bare_bones/bit.h"
#include "bare_bones/bitset.h"
#include "bare_bones/encoding.h"
#include "bare_bones/preprocess.h"
#include "series_data/data_storage.h"
#include "series_data/encoder/bit_sequence.h"

namespace series_data::unloading {

template <class DataStorage>
class Unloader {
 public:
  explicit Unloader(DataStorage& storage) : storage_(storage) {}

  template <class Stream>
  void create_snapshot(Stream& stream) {
    unloaded_chunks_.clear();

    const auto sequences = prepare_sequences();
    write_sequences(stream, sequences);
    write_bit_sequences_and_fill_unloaded_chunks(stream, sequences);
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

 private:
  struct ChunkSize {
    uint32_t ls_id;
    uint16_t trim_bytes;
    uint8_t chunk_id;
  };

  struct PreparedSequences {
    BareBones::Bitset ls_id_bitmap;
    EncodingChunkLengthSequence chunk_length_sequence;
    EncodingChunkIDSequence chunk_id_sequence;
    uint32_t total_bitseqs_size{};
    uint32_t reserved_stream_size{};
    uint32_t ls_id_count{};
  };

  DataStorage& storage_;
  BareBones::Vector<ChunkSize> unloaded_chunks_;

  [[nodiscard]] PreparedSequences prepare_sequences() const noexcept {
    PreparedSequences result{};

    result.ls_id_bitmap.reserve(get_unloadable_ls_id_size());

    for (uint32_t ls_id = 0; ls_id < storage_.open_chunks.size(); ++ls_id) {
      if (storage_.queried_series_bitmap.is_set(ls_id)) {
        continue;
      }

      const auto encoding_type = storage_.open_chunks[ls_id].encoding_state.encoding_type;
      if (is_unloadable_encoder(encoding_type) &&
          get_chunk_stream<chunk::DataChunk::Type::kOpen>(storage_, storage_.open_chunks[ls_id]).size_in_bits() >= BareBones::Bit::kByteBits) {
        push_series_to_sequences(result, ls_id);
      }
    }

    result.chunk_id_sequence.flush();
    result.chunk_length_sequence.flush();

    result.reserved_stream_size =
        calculate_stream_reserve_size(result.ls_id_bitmap, result.chunk_length_sequence, result.chunk_id_sequence, result.total_bitseqs_size);

    return result;
  }

  template <class Stream>
  static void write_sequences(Stream& stream, const PreparedSequences& sequences) noexcept {
    if constexpr (BareBones::concepts::has_reserve<Stream>) {
      stream.reserve(sequences.reserved_stream_size);
    }

    sequences.ls_id_bitmap.write_to(stream);
    sequences.chunk_length_sequence.data().write_to(stream);
    sequences.chunk_id_sequence.data().write_to(stream);
  }

  template <class Stream>
  void write_bit_sequences_and_fill_unloaded_chunks(Stream& stream, const PreparedSequences& sequences) noexcept {
    unloaded_chunks_.reserve(sequences.total_bitseqs_size);

    for (const auto ls_id : sequences.ls_id_bitmap) {
      auto& bitseq = get_chunk_stream<chunk::DataChunk::Type::kOpen>(storage_, storage_.open_chunks[ls_id]);
      const auto bitseq_size = BareBones::Bit::to_bytes(bitseq.size_in_bits());
      stream.write(reinterpret_cast<const char*>(bitseq.raw_bytes()), bitseq_size);

      unloaded_chunks_.emplace_back(ChunkSize{
          .ls_id = ls_id,
          .trim_bytes = static_cast<uint16_t>(bitseq_size),
          .chunk_id = static_cast<uint8_t>(storage_.get_open_chunk_index(ls_id)),
      });
    }

    if (sequences.total_bitseqs_size) {
      const auto& reserved_bytes = DataStorage::CompactBitSequence::reserved_bytes_for_reader();
      stream.write(reserved_bytes.data(), reserved_bytes.size());
    }
  }

  void push_series_to_sequences(PreparedSequences& sequences, uint32_t ls_id) const noexcept {
    sequences.ls_id_bitmap.set(ls_id);

    const auto& bitseq = get_chunk_stream<chunk::DataChunk::Type::kOpen>(storage_, storage_.open_chunks[ls_id]);
    const uint32_t bitseq_size = BareBones::Bit::to_bytes(bitseq.size_in_bits());
    sequences.chunk_length_sequence.push_back(bitseq_size);
    sequences.total_bitseqs_size += bitseq_size;

    const uint32_t chunk_id = get_open_chunk_id(ls_id);
    sequences.chunk_id_sequence.push_back(chunk_id);

    ++sequences.ls_id_count;
  }

  PROMPP_ALWAYS_INLINE static uint32_t calculate_stream_reserve_size(const BareBones::Bitset& ls_id_bitmap,
                                                                     const EncodingChunkLengthSequence& chunk_length_sequence,
                                                                     const EncodingChunkIDSequence& chunk_id_sequence,
                                                                     uint32_t total_bitseqs_size) noexcept {
    uint32_t reserved_stream_size = 0;
    reserved_stream_size += ls_id_bitmap.get_write_size();
    reserved_stream_size += chunk_id_sequence.data().get_write_size();
    reserved_stream_size += chunk_length_sequence.data().get_write_size();
    if (total_bitseqs_size) {
      reserved_stream_size += total_bitseqs_size + DataStorage::CompactBitSequence::reserved_bytes_for_reader().size();
    }
    return reserved_stream_size;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t get_unloadable_ls_id_size() const noexcept {
    if (storage_.queried_series_bitmap.empty()) {
      return storage_.open_chunks.size();
    }

    for (uint32_t ls_id_size = storage_.open_chunks.size(); ls_id_size != 0; --ls_id_size) {
      if (!storage_.queried_series_bitmap.is_set(ls_id_size - 1) && is_unloadable_encoder(storage_.open_chunks[ls_id_size - 1].encoding_state.encoding_type)) {
        return ls_id_size;
      }
    }

    return 0;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t get_open_chunk_id(uint32_t ls_id) const noexcept {
    if (const auto it = storage_.finalized_chunks.find(ls_id); it != storage_.finalized_chunks.end()) {
      return it->second.count();
    }
    return 0;
  }
};
}  // namespace series_data::unloading