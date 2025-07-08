#pragma once

#include "common.h"

#include "bare_bones/bit.h"
#include "bare_bones/bitset.h"
#include "bare_bones/encoding.h"
#include "bare_bones/preprocess.h"
#include "series_data/data_storage.h"
#include "series_data/encoder/bit_sequence.h"

namespace series_data::unloading {
class Unloader {
 public:
  explicit Unloader(DataStorage& storage) : storage_(storage) {}

  template <class Stream>
  void unload(Stream& stream) {
    const auto sequences = prepare_sequences();
    write_sequences(stream, sequences);
    write_bit_sequences(stream, sequences.ls_id_bitmap, sequences.total_bitseqs_size);
  }

 private:
  DataStorage& storage_;

  struct PreparedSequences {
    BareBones::Bitset ls_id_bitmap;
    EncodingChunkLengthSequence chunk_length_sequence;
    EncodingChunkIDSequence chunk_id_sequence;
    uint32_t total_bitseqs_size{};
    uint32_t reserved_stream_size{};
  };

  [[nodiscard]] PreparedSequences prepare_sequences() const noexcept {
    PreparedSequences result{};

    result.ls_id_bitmap.resize(get_unloadable_ls_id_size());

    for (uint32_t ls_id = 0; ls_id < storage_.open_chunks.size(); ++ls_id) {
      if (storage_.queried_series_bitmap.is_set(ls_id)) {
        continue;
      }

      const auto encoding_type = storage_.open_chunks[ls_id].encoding_state.encoding_type;
      if (is_unloadable_encoder(encoding_type)) {
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
  void write_bit_sequences(Stream& stream, const BareBones::Bitset& ls_id_bitmap, uint32_t total_bitseqs_size) noexcept {
    for (const auto ls_id : ls_id_bitmap) {
      auto& bitseq = get_open_chunk_stream(storage_, ls_id);
      const auto bitseq_size = BareBones::Bit::to_bytes(bitseq.size_in_bits());
      stream.write(reinterpret_cast<const char*>(bitseq.raw_bytes()), bitseq_size);
      bitseq.trim_lower_bytes(bitseq_size);
    }

    if (total_bitseqs_size) {
      const auto& reserved_bytes = encoder::CompactBitSequence::reserved_bytes_for_reader();
      stream.write(reserved_bytes.data(), reserved_bytes.size());
    }
  }

  void push_series_to_sequences(PreparedSequences& sequences, uint32_t ls_id) const noexcept {
    storage_.unloaded_series_bitmap.set(ls_id);

    sequences.ls_id_bitmap.set(ls_id);

    const auto& bitseq = get_open_chunk_stream(storage_, ls_id);
    const uint32_t bitseq_size = BareBones::Bit::to_bytes(bitseq.size_in_bits());
    sequences.chunk_length_sequence.push_back(bitseq_size);
    sequences.total_bitseqs_size += bitseq_size;

    const uint32_t chunk_id = get_open_chunk_id(ls_id);
    sequences.chunk_id_sequence.push_back(chunk_id);
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
      reserved_stream_size += total_bitseqs_size + encoder::CompactBitSequence::reserved_bytes_for_reader().size();
    }
    return reserved_stream_size;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t get_unloadable_ls_id_size() const noexcept {
    if (storage_.queried_series_bitmap.popcount() == 0) {
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