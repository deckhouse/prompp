#pragma once

#include "decoder.h"

namespace series_data {

class ChunkFinalizer {
 public:
  template <class DataStorage>
  static void finalize(DataStorage& storage, uint32_t ls_id, chunk::DataChunk& chunk) {
    (void)storage.timestamp_store.finalize(chunk.timestamp_id);
    const auto finalize_variant_encoder = [&storage, &chunk](auto& encoder, EncodingType encoding_type) PROMPP_LAMBDA_INLINE {
      const auto& finalized_stream = storage.finalized_data_streams.emplace_back(encoder.finalize_stream());
      storage.variant_encoders.erase(chunk.encoder.external_index, encoding_type);
      chunk.encoder.external_index = storage.finalized_data_streams.index_of(finalized_stream);
    };

    if (chunk.encoding_state.encoding_type == EncodingType::kAscInteger) [[likely]] {
      finalize_variant_encoder(storage.variant_encoders[chunk.encoder.external_index].asc_integer, chunk.encoding_state.encoding_type);
    } else if (chunk.encoding_state.encoding_type == EncodingType::kAscIntegerThenValuesGorilla) {
      finalize_variant_encoder(storage.variant_encoders[chunk.encoder.external_index].asc_integer_then_values_gorilla, chunk.encoding_state.encoding_type);
    } else if (chunk.encoding_state.encoding_type == EncodingType::kValuesGorilla) {
      finalize_variant_encoder(storage.variant_encoders[chunk.encoder.external_index].values_gorilla, chunk.encoding_state.encoding_type);
    }

    emplace_finalized_chunk(storage, ls_id, chunk);
    chunk.reset();
  }

 private:
  template <class DataStorage>
  PROMPP_ALWAYS_INLINE static void emplace_finalized_chunk(DataStorage& storage, uint32_t ls_id, const chunk::DataChunk& chunk) {
    storage.finalized_chunks.try_emplace(ls_id, storage.finalized_chunks_map_allocated_memory)
        .first->second.emplace(chunk, [&storage](const chunk::DataChunk& chunk) PROMPP_LAMBDA_INLINE {
          return Decoder::get_chunk_first_timestamp<chunk::DataChunk::Type::kFinalized>(storage, chunk);
        });
    storage.metrics->finalized_chunks().inc();
  }
};

}  // namespace series_data
