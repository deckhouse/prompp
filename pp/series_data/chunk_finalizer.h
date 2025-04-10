#pragma once

#include "data_storage.h"
#include "decoder.h"

namespace series_data {

class ChunkFinalizer {
 public:
  enum class FinalizeTimestampStateMode : uint8_t {
    kFinalize = 0,
    kFinalizeOrCopy,
  };

  PROMPP_ALWAYS_INLINE static void finalize(DataStorage& storage, uint32_t ls_id, chunk::DataChunk& chunk) {
    if (chunk.encoding_state.encoding_type == EncodingType::kGorilla) [[unlikely]] {
      finalize(storage, ls_id, chunk, encoder::timestamp::State::kInvalidId);
    } else {
      finalize_timestamp_and_chunk_separately<FinalizeTimestampStateMode::kFinalize>(storage, ls_id, chunk);
    }
  }

  static void finalize(DataStorage& storage, uint32_t ls_id, chunk::DataChunk& chunk, uint32_t finalized_timestamp_stream_id) {
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
    } else if (chunk.encoding_state.encoding_type == EncodingType::kGorilla) {
      const auto& finalized_stream = storage.finalized_data_streams.emplace_back(storage.gorilla_encoders[chunk.encoder.external_index].finalize_stream());
      storage.gorilla_encoders.erase(chunk.encoder.external_index);
      chunk.encoder.external_index = storage.finalized_data_streams.index_of(finalized_stream);
    }

    chunk.timestamp_encoder_state_id = finalized_timestamp_stream_id;
    emplace_finalized_chunk(storage, ls_id, chunk);
    chunk.reset();
  }

  template <FinalizeTimestampStateMode mode>
  PROMPP_ALWAYS_INLINE static void finalize_timestamp_and_chunk_separately(DataStorage& storage, uint32_t ls_id, chunk::DataChunk& chunk) {
    if (!finalize_if_timestamp_finalized(storage, ls_id, chunk)) [[likely]] {
      finalize(storage, ls_id, chunk, finalize_timestamp<mode>(storage, chunk));
    }
  }

  PROMPP_ALWAYS_INLINE static bool finalize_if_timestamp_finalized(DataStorage& storage, uint32_t ls_id, chunk::DataChunk& chunk) {
    if (const auto finalized_timestamp_stream_id = storage.timestamp_encoder.process_finalized(chunk.timestamp_encoder_state_id);
        finalized_timestamp_stream_id != encoder::timestamp::State::kInvalidId) [[unlikely]] {
      ++storage.finalized_timestamp_streams[finalized_timestamp_stream_id].reference_count;
      finalize(storage, ls_id, chunk, finalized_timestamp_stream_id);
      return true;
    }

    return false;
  }

 private:
  PROMPP_ALWAYS_INLINE static void emplace_finalized_chunk(DataStorage& storage, uint32_t ls_id, const chunk::DataChunk& chunk) {
    storage.finalized_chunks.try_emplace(ls_id, storage.finalized_chunks_map_allocated_memory)
        .first->second.emplace(chunk, [&storage](const chunk::DataChunk& chunk) PROMPP_LAMBDA_INLINE {
          return Decoder::get_chunk_first_timestamp<chunk::DataChunk::Type::kFinalized>(storage, chunk);
        });
  }

  template <FinalizeTimestampStateMode mode>
  PROMPP_ALWAYS_INLINE static encoder::timestamp::State::Id finalize_timestamp(DataStorage& storage, chunk::DataChunk& chunk) {
    auto& finalized_stream = storage.finalized_timestamp_streams.emplace_back();
    const auto finalized_stream_id = storage.finalized_timestamp_streams.index_of(finalized_stream);

    if constexpr (mode == FinalizeTimestampStateMode::kFinalize) {
      storage.timestamp_encoder.finalize(chunk.timestamp_encoder_state_id, finalized_stream.stream, finalized_stream_id);
    } else {
      storage.timestamp_encoder.finalize_or_copy(chunk.timestamp_encoder_state_id, finalized_stream.stream, finalized_stream_id);
    }

    return finalized_stream_id;
  }
};

}  // namespace series_data