#pragma once

#include "bare_bones/bit.h"
#include "bare_bones/bitset.h"
#include "bare_bones/encoding.h"
#include "series_data/data_storage.h"
#include "series_data/encoder/bit_sequence.h"

namespace series_data::unloading {
class Unloader {
  using EncodingChunkLengthSequence =
      BareBones::EncodedSequence<BareBones::Encoding::DeltaDeltaZigZag<BareBones::StreamVByte::CompactSequence<BareBones::StreamVByte::Codec0124Frequent0>>>;
  using EncodingChunkIDSequence =
      BareBones::EncodedSequence<BareBones::Encoding::RLE<BareBones::StreamVByte::CompactSequence<BareBones::StreamVByte::Codec0124Frequent0>>>;

 public:
  explicit Unloader(DataStorage& storage) : storage_(storage) {}

  template <class Stream>
  void unload(Stream& stream) {
    const auto sequences = prepare_sequences();
    write_sequences(stream, sequences.ls_id_bitmap, sequences.chunk_length_sequence, sequences.chunk_id_sequence);
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
    uint32_t total_bitseqs_size;
  };

  [[nodiscard]] PreparedSequences prepare_sequences() const noexcept {
    PreparedSequences result{};
    result.total_bitseqs_size = 0;

    for (const auto ls_id : storage_.unused_series_bitmap) {
      const auto encoding_type = storage_.open_chunks[ls_id].encoding_state.encoding_type;
      if (!storage_.open_chunks[ls_id].is_empty() && is_unloadable_encoder(encoding_type)) {
        result.ls_id_bitmap.resize(ls_id + 1);
        result.ls_id_bitmap.set(ls_id);

        const auto& bitseq = get_open_chunk_stream(ls_id);
        const uint32_t bitseq_size = BareBones::Bit::to_bytes(bitseq.size_in_bits());
        result.chunk_length_sequence.push_back(bitseq_size);
        result.total_bitseqs_size += bitseq_size;

        const uint32_t chunk_id = get_open_chunk_id(ls_id);
        result.chunk_id_sequence.push_back(chunk_id);
      }
    }

    result.chunk_id_sequence.flush();
    result.chunk_length_sequence.flush();

    return result;
  }

  template <class Stream>
  static void write_sequences(Stream& stream,
                              const BareBones::Bitset& ls_id_bitmap,
                              const EncodingChunkLengthSequence& chunk_length_sequence,
                              const EncodingChunkIDSequence& chunk_id_sequence) noexcept {
    ls_id_bitmap.write_to(stream);

    chunk_length_sequence.data().write_to(stream);

    chunk_id_sequence.data().write_to(stream);
  }

  template <class Stream>
  void write_bit_sequences(Stream& stream, const BareBones::Bitset& ls_id_bitmap, uint32_t total_bitseqs_size) noexcept {
    stream.write(reinterpret_cast<char*>(&total_bitseqs_size), sizeof(total_bitseqs_size));

    for (const auto ls_id : ls_id_bitmap) {
      auto& bitseq = get_open_chunk_stream(ls_id);
      const auto bitseq_size = BareBones::Bit::to_bytes(bitseq.size_in_bits());
      stream.write(reinterpret_cast<const char*>(bitseq.raw_bytes()), bitseq_size);
      bitseq.trim_lower_bytes(bitseq_size);
    }

    if (total_bitseqs_size) {
      const auto& reserved_bytes = encoder::CompactBitSequence::reserved_bytes_for_reader();
      stream.write(reserved_bytes.data(), reserved_bytes.size());
    }
  }

  [[nodiscard]] encoder::CompactBitSequence& get_open_chunk_stream(uint32_t ls_id) const noexcept {
    using enum EncodingType;

    const auto& chunk = storage_.open_chunks[ls_id];
    const auto encoding_type = storage_.open_chunks[ls_id].encoding_state.encoding_type;

    if (encoding_type == kAscInteger) {
      return storage_.get_asc_integer_stream<chunk::DataChunk::Type::kOpen>(chunk.encoder.external_index);
    }
    if (encoding_type == kValuesGorilla) {
      return storage_.get_values_gorilla_stream<chunk::DataChunk::Type::kOpen>(chunk.encoder.external_index);
    }
    return storage_.get_asc_integer_then_values_gorilla_stream<chunk::DataChunk::Type::kOpen>(chunk.encoder.external_index);
  }

  [[nodiscard]] uint32_t get_open_chunk_id(uint32_t ls_id) const noexcept {
    if (const auto it = storage_.finalized_chunks.find(ls_id); it != storage_.finalized_chunks.end()) {
      return it->second.count();
    }
    return 0;
  }
};
}  // namespace series_data::unloading