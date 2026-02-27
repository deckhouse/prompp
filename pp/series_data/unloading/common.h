#pragma once

#include "bare_bones/encoding.h"
#include "bare_bones/preprocess.h"
#include "series_data/data_storage.h"

namespace series_data::unloading {

using EncodingChunkLengthSequence =
    BareBones::EncodedSequence<BareBones::Encoding::DeltaDeltaZigZag<BareBones::StreamVByte::CompactSequence<BareBones::StreamVByte::Codec0124Frequent0>>>;
using EncodingChunkIDSequence =
    BareBones::EncodedSequence<BareBones::Encoding::RLE<BareBones::StreamVByte::CompactSequence<BareBones::StreamVByte::Codec0124Frequent0>>>;

template <chunk::DataChunk::Type ChunkType>
[[nodiscard]] PROMPP_ALWAYS_INLINE DataStorage::CompactBitSequence& get_chunk_stream(DataStorage& storage, const chunk::DataChunk& chunk) noexcept {
  using enum EncodingType;

  const auto encoding_type = chunk.encoding_state.encoding_type;

  if (encoding_type == kAscInteger) {
    return storage.get_asc_integer_stream<ChunkType>(chunk.encoder.external_index);
  }
  if (encoding_type == kValuesGorilla) {
    return storage.get_values_gorilla_stream<ChunkType>(chunk.encoder.external_index);
  }
  return storage.get_asc_integer_then_values_gorilla_stream<ChunkType>(chunk.encoder.external_index);
}

[[nodiscard]] PROMPP_ALWAYS_INLINE DataStorage::CompactBitSequence& get_chunk_stream(DataStorage& storage,
                                                                                     const chunk::DataChunk& chunk,
                                                                                     bool is_open) noexcept {
  if (is_open) {
    return get_chunk_stream<chunk::DataChunk::Type::kOpen>(storage, chunk);
  }

  return get_chunk_stream<chunk::DataChunk::Type::kFinalized>(storage, chunk);
}

[[nodiscard]] PROMPP_ALWAYS_INLINE DataStorage::CompactBitSequence& get_chunk_stream(DataStorage& storage, uint32_t ls_id, uint8_t chunk_id) noexcept {
  const auto& chunk_data = std::ranges::next(DataStorage::SeriesChunkIterator{&storage, ls_id}, chunk_id, DataStorage::SeriesChunks::end());
  return get_chunk_stream(storage, chunk_data->chunk(), chunk_data->is_open());
}

}  // namespace series_data::unloading