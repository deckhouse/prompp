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
    using enum EncodingType;

    EncodingChunkLengthSequence chunk_length_sequence{};
    EncodingChunkIDSequence chunk_id_sequence{};
    BareBones::Bitset ls_id_bitmap{};

    uint32_t bitseqs_size_in_bytes = 0;

    for (const auto ls_id : storage_.unused_series_bitmap) {
      const auto encoding_type = storage_.open_chunks[ls_id].encoding_state.encoding_type;
      if (!storage_.open_chunks[ls_id].is_empty() && is_unloadable_encoder(encoding_type)) {
        ls_id_bitmap.resize(ls_id + 1);
        ls_id_bitmap.set(ls_id);

        const auto& bitseq = get_open_chunk_stream(ls_id);
        const uint32_t bitseq_filled_bytes_count = std::min(bitseq.size_in_bytes(), BareBones::Bit::to_bytes(bitseq.size_in_bits()));
        chunk_length_sequence.push_back(bitseq_filled_bytes_count);
        bitseqs_size_in_bytes += bitseq_filled_bytes_count;

        const uint32_t chunk_id = get_open_chunk_id(ls_id);
        chunk_id_sequence.push_back(chunk_id);
      }
    }

    ls_id_bitmap.write_to(stream);

    chunk_length_sequence.flush();
    std::cout << "chunk_length_sequence.data().size(): " << chunk_length_sequence.data().size() << '\n';
    chunk_length_sequence.data().write_to(stream);
    for (auto x : chunk_length_sequence) {
      std::cout << int(x) << ' ';
    }
    std::cout << '\n';

    chunk_id_sequence.flush();
    std::cout << "chunk_id_sequence.data().size(): " << chunk_id_sequence.data().size() << '\n';
    chunk_id_sequence.data().write_to(stream);
    for (auto x : chunk_id_sequence) {
      std::cout << int(x) << ' ';
    }
    std::cout << '\n';

    stream.write(reinterpret_cast<char*>(&bitseqs_size_in_bytes), sizeof(bitseqs_size_in_bytes));

    for (const auto ls_id : ls_id_bitmap) {
      auto& bitseq = get_open_chunk_stream(ls_id);
      const auto bitseq_filled_bytes_count = std::min(bitseq.size_in_bytes(), BareBones::Bit::to_bytes(bitseq.size_in_bits()));
      stream.write(reinterpret_cast<const char*>(bitseq.raw_bytes()), bitseq_filled_bytes_count);
      bitseq.trim_lower_bytes(bitseq_filled_bytes_count);
    }
  }

  static constexpr uint32_t get_empty_unloader_size_in_bytes() noexcept {
    return sizeof(uint32_t) + BareBones::Bitset{}.allocated_memory() + EncodingChunkLengthSequence{}.data().size() + sizeof(uint32_t) +
           EncodingChunkIDSequence{}.data().size() + sizeof(uint32_t) + sizeof(uint32_t);
  }

 private:
  DataStorage& storage_;

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
    // encoding_type == kAscIntegerThenValuesGorilla
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