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
    write_sequences(stream, sequences.ls_id_bitmap, sequences.chunk_length_sequence, sequences.chunk_id_sequence, sequences.reserved_stream_size);
    write_bit_sequences(stream, sequences.ls_id_bitmap, sequences.total_bitseqs_size);
  }

  static constexpr uint32_t get_empty_unloader_size_in_bytes() noexcept {
    return sizeof(uint32_t) + BareBones::Bitset{}.allocated_memory() + EncodingChunkLengthSequence{}.data().size() + sizeof(uint32_t) + sizeof(uint32_t) +
           EncodingChunkIDSequence{}.data().size() + sizeof(uint32_t) + sizeof(uint32_t) + sizeof(uint32_t);
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

    result.ls_id_bitmap.resize(storage_.unloaded_series_bitmap.isEmpty() ? 0 : storage_.unloaded_series_bitmap.maximum() + 1);

    for (const auto ls_id : storage_.unloaded_series_bitmap) {
      const auto encoding_type = storage_.open_chunks[ls_id].encoding_state.encoding_type;
      if (is_unloadable_encoder(encoding_type)) {
        result.ls_id_bitmap.set(ls_id);

        const auto& bitseq = get_open_chunk_stream(storage_, ls_id);
        const uint32_t bitseq_size = BareBones::Bit::to_bytes(bitseq.size_in_bits());
        result.chunk_length_sequence.push_back(bitseq_size);
        result.total_bitseqs_size += bitseq_size;

        const uint32_t chunk_id = get_open_chunk_id(ls_id);
        result.chunk_id_sequence.push_back(chunk_id);
      }
    }

    result.chunk_id_sequence.flush();
    result.chunk_length_sequence.flush();

    result.reserved_stream_size =
        calculate_stream_reserve_size(result.ls_id_bitmap, result.chunk_length_sequence, result.chunk_id_sequence, result.total_bitseqs_size);

    return result;
  }

  template <class Stream>
  static void write_sequences(Stream& stream,
                              const BareBones::Bitset& ls_id_bitmap,
                              const EncodingChunkLengthSequence& chunk_length_sequence,
                              const EncodingChunkIDSequence& chunk_id_sequence,
                              const uint32_t reserved_size) noexcept {
    if constexpr (BareBones::concepts::has_reserve<Stream>) {
      stream.reserve(reserved_size);
    }

    ls_id_bitmap.write_to(stream);

    chunk_length_sequence.data().write_to(stream);

    chunk_id_sequence.data().write_to(stream);
  }

  template <class Stream>
  void write_bit_sequences(Stream& stream, const BareBones::Bitset& ls_id_bitmap, uint32_t total_bitseqs_size) noexcept {
    stream.write(reinterpret_cast<char*>(&total_bitseqs_size), sizeof(total_bitseqs_size));

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

  PROMPP_ALWAYS_INLINE static uint32_t calculate_stream_reserve_size(const BareBones::Bitset& ls_id_bitmap,
                                                                     const EncodingChunkLengthSequence& chunk_length_sequence,
                                                                     const EncodingChunkIDSequence& chunk_id_sequence,
                                                                     uint32_t total_bitseqs_size) noexcept {
    uint32_t reserved_stream_size = 0;
    reserved_stream_size += BareBones::Bit::to_ceil_units<uint64_t>(ls_id_bitmap.size()) * sizeof(uint64_t) + sizeof(uint32_t);
    reserved_stream_size += chunk_id_sequence.data().size_in_bytes() + 2 * sizeof(uint32_t);
    reserved_stream_size += chunk_length_sequence.data().size_in_bytes() + 2 * sizeof(uint32_t);
    if (total_bitseqs_size) {
      reserved_stream_size += total_bitseqs_size + encoder::CompactBitSequence::reserved_bytes_for_reader().size();
    }
    return reserved_stream_size;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t get_open_chunk_id(uint32_t ls_id) const noexcept {
    if (const auto it = storage_.finalized_chunks.find(ls_id); it != storage_.finalized_chunks.end()) {
      return it->second.count();
    }
    return 0;
  }
};
}  // namespace series_data::unloading