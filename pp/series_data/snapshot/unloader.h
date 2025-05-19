#pragma once

#include <roaring/cpp/roaring.hh>

#include "bare_bones/encoding.h"
#include "series_data/data_storage.h"
#include "series_data/encoder/bit_sequence.h"

namespace series_data::snapshot {
class Unloader {
  using EncodingChunkLengthSequence =
      BareBones::EncodedSequence<BareBones::Encoding::DeltaDeltaZigZag<BareBones::StreamVByte::Sequence<BareBones::StreamVByte::Codec0124Frequent0>>>;
  using EncodingChunkIDSequence =
      BareBones::EncodedSequence<BareBones::Encoding::RLE<BareBones::StreamVByte::Sequence<BareBones::StreamVByte::Codec0124Frequent0>>>;

 public:
  explicit Unloader(DataStorage& storage) : storage_(storage) {}

  template <class Stream>
  void unload(Stream& stream) {
    using enum EncodingType;

    EncodingChunkLengthSequence chunk_length_sequence{};
    EncodingChunkIDSequence chunk_id_sequence{};
    roaring::Roaring ls_id_bitmap{};

    size_t bitseqs_seize_in_bytes = 0;

    for (const auto ls_id : storage_.unused_series_bitmap) {
      const auto encoding_type = storage_.open_chunks[ls_id].encoding_state.encoding_type;
      if (!storage_.open_chunks[ls_id].is_empty() && is_unloadable_encoder(encoding_type)) {
        ls_id_bitmap.add(ls_id);
        const auto& bitseq = get_encoder_stream(ls_id);
        length_sequence.push_back(bitseq.size_in_bits());
        bitseqs_seize_in_bytes += bitseq.size_in_bytes();
      }
    }

    ls_id_bitmap.runOptimize();
    ls_id_bitmap.shrinkToFit();
    size_t expected_size_in_bytes = ls_id_bitmap.getSizeInBytes();
    std::vector<char> buffer(expected_size_in_bytes);
    size_t size_in_bytes = ls_id_bitmap.write(buffer.data());
    assert(expected_size_in_bytes == size_in_bytes);
    stream << size_in_bytes;
    stream.write(buffer.data(), size_in_bytes);

    length_sequence.flush();
    stream << length_sequence;

    auto& fin_bitmap = storage_.finalized_chunks_since_last_unloading;
    fin_bitmap.runOptimize();
    fin_bitmap.shrinkToFit();
    expected_size_in_bytes = fin_bitmap.getSizeInBytes();
    buffer.resize(expected_size_in_bytes);
    size_in_bytes = fin_bitmap.write(buffer.data());
    assert(expected_size_in_bytes == size_in_bytes);
    stream << size_in_bytes;
    stream.write(buffer.data(), size_in_bytes);

    stream << bitseqs_seize_in_bytes;

    for (const auto ls_id : ls_id_bitmap) {
      auto& bitseq = get_encoder_stream(ls_id);
      bitseq.write_to(stream);
    }
  }

 private:
  DataStorage& storage_;

  [[nodiscard]] encoder::CompactBitSequence& get_encoder_stream(uint32_t ls_id) const noexcept {
    using enum EncodingType;

    const auto& chunk = storage_.open_chunks[ls_id];
    const auto encoding_type = storage_.open_chunks[ls_id].encoding_state.encoding_type;

    if (encoding_type == kAscInteger) {
      if (storage_.finalized_chunks_since_last_unloading.contains(ls_id)) {
        return storage_.get_asc_integer_stream<chunk::DataChunk::Type::kFinalized>(chunk.encoder.external_index);
      } else {
        return storage_.get_asc_integer_stream<chunk::DataChunk::Type::kOpen>(chunk.encoder.external_index);
      }
    } else if (encoding_type == kValuesGorilla) {
      if (storage_.finalized_chunks_since_last_unloading.contains(ls_id)) {
        return storage_.get_values_gorilla_stream<chunk::DataChunk::Type::kFinalized>(chunk.encoder.external_index);
      } else {
        return storage_.get_values_gorilla_stream<chunk::DataChunk::Type::kOpen>(chunk.encoder.external_index);
      }
    } else {  // encoding_type == kAscIntegerThenValuesGorilla
      if (storage_.finalized_chunks_since_last_unloading.contains(ls_id)) {
        return storage_.get_asc_integer_then_values_gorilla_stream<chunk::DataChunk::Type::kFinalized>(chunk.encoder.external_index);
      } else {
        return storage_.get_asc_integer_then_values_gorilla_stream<chunk::DataChunk::Type::kOpen>(chunk.encoder.external_index);
      }
    }
  }
};
}  // namespace series_data::snapshot