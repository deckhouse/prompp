#pragma once

#include "bare_bones/encoding.h"
#include "bare_bones/preprocess.h"
#include "series_data/data_storage.h"

namespace series_data::unloading {

using EncodingChunkLengthSequence =
    BareBones::EncodedSequence<BareBones::Encoding::DeltaDeltaZigZag<BareBones::StreamVByte::CompactSequence<BareBones::StreamVByte::Codec0124Frequent0>>>;
using EncodingChunkIDSequence =
    BareBones::EncodedSequence<BareBones::Encoding::RLE<BareBones::StreamVByte::CompactSequence<BareBones::StreamVByte::Codec0124Frequent0>>>;

[[nodiscard]] PROMPP_ALWAYS_INLINE encoder::CompactBitSequence& get_open_chunk_stream(DataStorage& storage, uint32_t ls_id) noexcept {
  using enum EncodingType;

  const auto& chunk = storage.open_chunks[ls_id];
  const auto encoding_type = storage.open_chunks[ls_id].encoding_state.encoding_type;

  if (encoding_type == kAscInteger) {
    return storage.get_asc_integer_stream<chunk::DataChunk::Type::kOpen>(chunk.encoder.external_index);
  }
  if (encoding_type == kValuesGorilla) {
    return storage.get_values_gorilla_stream<chunk::DataChunk::Type::kOpen>(chunk.encoder.external_index);
  }
  return storage.get_asc_integer_then_values_gorilla_stream<chunk::DataChunk::Type::kOpen>(chunk.encoder.external_index);
}

}  // namespace series_data::unloading