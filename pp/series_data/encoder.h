#pragma once

#include <cassert>

#include "bare_bones/gorilla.h"
#include "bare_bones/preprocess.h"
#include "chunk_finalizer.h"
#include "common.h"
#include "concepts.h"
#include "data_storage.h"
#include "encoder/encoder_variant.h"
#include "series_data/encoder/timestamp/encoder.h"
#include "series_data/encoder/timestamp/state.h"

namespace series_data {

template <uint8_t kSamplesPerChunk = kSamplesPerChunkDefault>
class Encoder {
 public:
  Encoder(DataStorage& storage) : storage_(storage) {}

  DataStorage& storage() noexcept { return storage_; }

  PROMPP_ALWAYS_INLINE void encode(uint32_t ls_id, int64_t timestamp, double value) {
    ++storage_.samples_count;

    if (storage_.open_chunks.size() <= ls_id) [[unlikely]] {
      storage_.open_chunks.resize(ls_id + 1);
      storage_.queried_series_bitmap.resize(ls_id + 1);
      storage_.unloaded_series_bitmap.resize(ls_id + 1);
    }

    encode(ls_id, timestamp, value, storage_.open_chunks[ls_id]);
  }

  PROMPP_ALWAYS_INLINE void encode(uint32_t ls_id, int64_t timestamp, double value, chunk::DataChunk& chunk) { encode_impl(ls_id, timestamp, value, chunk); }

 private:
  DataStorage& storage_;

  void encode_impl(uint32_t ls_id, int64_t timestamp, double value, chunk::DataChunk& chunk) {
    if (should_skip_stalenan(value, chunk)) [[unlikely]] {
      return;
    }

    if (!handle_timestamp_update(ls_id, timestamp, value, chunk)) {
      return;
    }

    encode_value(ls_id, chunk, timestamp, value);
    update_encoder_timestamp(chunk, timestamp);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static bool should_skip_stalenan(double value, const chunk::DataChunk& chunk) {
    return BareBones::Encoding::Gorilla::isstalenan(value) && chunk.encoding_state.has_last_stalenan;
  }

  bool handle_timestamp_update(uint32_t ls_id, int64_t timestamp, double value, chunk::DataChunk& chunk) {
    if (chunk.encoding_state.encoding_type == EncodingType::kGorilla) {
      return process_gorilla_encoding(ls_id, timestamp, value, chunk);
    }

    if (chunk.timestamp_encoder_state_id != encoder::timestamp::State::kInvalidId) {
      return process_value_timestamp_encoding(ls_id, timestamp, value, chunk);
    }

    return true;
  }

  PROMPP_ALWAYS_INLINE bool process_gorilla_encoding(uint32_t ls_id, int64_t timestamp, double value, chunk::DataChunk& chunk) {
    const auto& encoder = storage_.gorilla_encoders[chunk.encoder.external_index];

    if (timestamp > encoder.timestamp()) [[likely]] {
      if (encoder.stream().count() >= kSamplesPerChunk) [[unlikely]] {
        ChunkFinalizer::finalize(storage_, ls_id, chunk);
      }
      return true;
    }

    handle_outdated_sample(ls_id, timestamp, value, encoder.timestamp());
    return false;
  }

  PROMPP_ALWAYS_INLINE bool process_value_timestamp_encoding(uint32_t ls_id, int64_t timestamp, double value, chunk::DataChunk& chunk) {
    const auto& state = storage_.timestamp_encoder.get_state(chunk.timestamp_encoder_state_id);

    if (timestamp > state.timestamp()) [[likely]] {
      if (!ChunkFinalizer::finalize_if_timestamp_finalized(storage_, ls_id, chunk)) [[likely]] {
        if (state.stream_data.stream.count() >= kSamplesPerChunk) [[unlikely]] {
          ChunkFinalizer::finalize(storage_, ls_id, chunk);
        }
      }
      return true;
    }

    handle_outdated_sample(ls_id, timestamp, value, state.timestamp());
    return false;
  }

  PROMPP_ALWAYS_INLINE void handle_outdated_sample(uint32_t ls_id, int64_t timestamp, double value, int64_t last_timestamp) {
    if (timestamp < last_timestamp) {
      ++storage_.outdated_samples_count;

      if (auto it = storage_.outdated_chunks.try_emplace(ls_id, timestamp, value); !it.second) {
        it.first->second.encode(timestamp, value);
      } else {
        ++storage_.outdated_chunks_count;
      }
    }
  }

  PROMPP_ALWAYS_INLINE void update_encoder_timestamp(chunk::DataChunk& chunk, int64_t timestamp) const {
    if (chunk.encoding_state.encoding_type != EncodingType::kGorilla) {
      chunk.timestamp_encoder_state_id = storage_.timestamp_encoder.encode(chunk.timestamp_encoder_state_id, timestamp);
    }
  }

  void encode_value(uint32_t ls_id, chunk::DataChunk& chunk, int64_t timestamp, double value) const {
    switch (chunk.encoding_state.encoding_type) {
      case EncodingType::kUnknown: {
        return init_encoder(chunk, value);
      }
      case EncodingType::kUint32Constant: {
        return handle_inplace_constant_encoder(chunk, timestamp, chunk.encoder.uint32_constant, value);
      }
      case EncodingType::kFloat32Constant: {
        return handle_inplace_constant_encoder(chunk, timestamp, chunk.encoder.float32_constant, value);
      }
      case EncodingType::kDoubleConstant: {
        return handle_double_constant_encoder(chunk, timestamp, value);
      }
      case EncodingType::kTwoDoubleConstant: {
        return handle_two_double_constant_encoder(ls_id, chunk, timestamp, value);
      }
      case EncodingType::kAscInteger: {
        return handle_asc_integer_encoder(chunk, value);
      }
      case EncodingType::kAscIntegerThenValuesGorilla: {
        return handle_asc_integer_then_values_gorilla_encoder(chunk, value);
      }
      case EncodingType::kValuesGorilla: {
        return handle_values_gorilla_encoder(chunk, value);
      }
      case EncodingType::kGorilla: {
        return handle_gorilla_encoder(chunk, timestamp, value);
      }
      default: {
        assert(chunk.encoding_state.encoding_type != EncodingType::kUint32Constant);
      }
    }
  }

  void init_encoder(chunk::DataChunk& chunk, double value) const {
    if (encoder::value::Uint32ConstantEncoder::can_be_encoded(value)) [[likely]] {
      chunk.encoding_state.encoding_type = EncodingType::kUint32Constant;
      std::construct_at(&chunk.encoder.uint32_constant, encoder::value::Uint32ConstantEncoder(value));
    } else if (encoder::value::Float32ConstantEncoder::can_be_encoded(value)) [[unlikely]] {
      chunk.encoding_state.encoding_type = EncodingType::kFloat32Constant;
      std::construct_at(&chunk.encoder.float32_constant, encoder::value::Float32ConstantEncoder(value));
    } else {
      switch_to_double_constant_encoder(chunk, value);
    }
  }

  template <typename EncoderType>
  PROMPP_ALWAYS_INLINE void handle_inplace_constant_encoder(chunk::DataChunk& chunk, int64_t timestamp, EncoderType& encoder, double value) const {
    if (!encoder.encode(chunk.encoding_state, value)) {
      switch_from_constant(chunk, timestamp, encoder.value(), value);
    }
  }

  PROMPP_ALWAYS_INLINE void handle_double_constant_encoder(chunk::DataChunk& chunk, int64_t timestamp, double value) const {
    if (const auto& encoder = storage_.variant_encoders[chunk.encoder.external_index].double_constant; !encoder.encode(chunk.encoding_state, value)) {
      const auto encoder_id = chunk.encoder.external_index;

      const auto encoder_copy = storage_.variant_encoders[encoder_id].double_constant;
      storage_.variant_encoders.erase(encoder_id, EncodingType::kDoubleConstant);

      switch_from_constant(chunk, timestamp, encoder_copy.value(), value);
    }
  }

  PROMPP_ALWAYS_INLINE void handle_two_double_constant_encoder(uint32_t ls_id, chunk::DataChunk& chunk, int64_t timestamp, double value) const {
    if (const auto& encoder = storage_.variant_encoders[chunk.encoder.external_index].two_double_constant; !encoder.encode(chunk.encoding_state, value)) {
      const auto encoder_id = chunk.encoder.external_index;
      const bool was_last_stalenan = chunk.encoding_state.has_last_stalenan;

      const auto encoder_copy = storage_.variant_encoders[encoder_id].two_double_constant;
      storage_.variant_encoders.erase(encoder_id, EncodingType::kTwoDoubleConstant);

      switch_from_two_double_constant(chunk, timestamp, encoder_copy.value1(), encoder_copy.value1_count(), encoder_copy.value2(), value);

      if (was_last_stalenan) [[unlikely]] {
        encode_value(ls_id, chunk, timestamp, value);
      }
    }
  }

  PROMPP_ALWAYS_INLINE void handle_asc_integer_encoder(chunk::DataChunk& chunk, double value) const {
    if (!storage_.variant_encoders[chunk.encoder.external_index].asc_integer.encode(chunk.encoding_state, value)) {
      auto encoder = std::move(storage_.variant_encoders[chunk.encoder.external_index].asc_integer);
      storage_.variant_encoders.erase(chunk.encoder.external_index, EncodingType::kAscInteger);
      switch_to_asc_integer_then_values_gorilla(chunk, std::move(encoder), value);
    }
  }

  PROMPP_ALWAYS_INLINE void handle_values_gorilla_encoder(chunk::DataChunk& chunk, double value) const {
    storage_.variant_encoders[chunk.encoder.external_index].values_gorilla.encode(chunk.encoding_state, value);
  }

  PROMPP_ALWAYS_INLINE void handle_asc_integer_then_values_gorilla_encoder(chunk::DataChunk& chunk, double value) const {
    storage_.variant_encoders[chunk.encoder.external_index].asc_integer_then_values_gorilla.encode(chunk.encoding_state, value);
  }

  PROMPP_ALWAYS_INLINE void handle_gorilla_encoder(chunk::DataChunk& chunk, int64_t timestamp, double value) const {
    storage_.gorilla_encoders[chunk.encoder.external_index].encode(chunk.encoding_state, timestamp, value);
  }

  PROMPP_ALWAYS_INLINE void switch_from_constant(chunk::DataChunk& chunk, int64_t timestamp, double const_value, double value) const {
    const uint8_t const_value_count = storage_.timestamp_encoder.get_stream(chunk.timestamp_encoder_state_id).count() - chunk.encoding_state.has_last_stalenan;

    encoder::value::ConstantValue v1{.value = const_value, .count = const_value_count};
    encoder::value::ConstantValue v2{.value = value, .count = 1};
    encoder::value::ConstantValue v3{};

    if (v1.is_stalenan()) [[unlikely]] {
      ++v1.count;
    } else if (chunk.encoding_state.has_last_stalenan) [[unlikely]] {
      v3 = std::exchange(v2, encoder::value::ConstantValue{.value = BareBones::Encoding::Gorilla::STALE_NAN, .count = 1});
    }

    switch_from_constant_impl(chunk, timestamp, v1, v2, v3);
  }

  void switch_from_two_double_constant(chunk::DataChunk& chunk, int64_t timestamp, double value1, uint8_t value1_count, double value2, double value) const {
    const uint8_t value2_count =
        storage_.timestamp_encoder.get_stream(chunk.timestamp_encoder_state_id).count() - value1_count - chunk.encoding_state.has_last_stalenan;

    const encoder::value::ConstantValue v1{.value = value1, .count = value1_count};
    const encoder::value::ConstantValue v2{.value = value2, .count = value2_count};
    encoder::value::ConstantValue v3{.value = BareBones::Encoding::Gorilla::STALE_NAN, .count = 1};

    if (!chunk.encoding_state.has_last_stalenan) [[likely]] {
      v3.value = value;
    }

    switch_from_constant_impl(chunk, timestamp, v1, v2, v3);
  }

  void switch_from_constant_impl(chunk::DataChunk& chunk,
                                 int64_t timestamp,
                                 const encoder::value::ConstantValue& v1,
                                 const encoder::value::ConstantValue& v2,
                                 const encoder::value::ConstantValue& v3) const {
    if (!v3.has_value()) [[likely]] {
      switch_to_two_constant_encoder(chunk, v1, v2.value);
    } else if (encoder::value::AscIntegerEncoder::can_be_encoded(v1.value, v1.count, v2.value, v3.value)) {
      switch_to_asc_integer(chunk, v1, v2, v3);
    } else if (!storage_.timestamp_encoder.is_unique_state(chunk.timestamp_encoder_state_id)) {
      switch_to_values_gorilla(chunk, v1, v2, v3);
    } else {
      switch_to_gorilla(chunk, timestamp, v1, v2, v3);
    }
  }

  PROMPP_ALWAYS_INLINE void switch_to_double_constant_encoder(chunk::DataChunk& chunk, double value) const {
    chunk.encoding_state.encoding_type = EncodingType::kDoubleConstant;
    chunk.encoding_state.has_last_stalenan = BareBones::Encoding::Gorilla::isstalenan(value);
    auto& encoder = storage_.variant_encoders.emplace_back();
    encoder.construct<EncodingType::kDoubleConstant>(value);
    chunk.encoder.external_index = storage_.variant_encoders.index_of(encoder);
  }

  PROMPP_ALWAYS_INLINE void switch_to_two_constant_encoder(chunk::DataChunk& chunk, const encoder::value::ConstantValue& v1, double value2) const {
    auto& encoder = storage_.variant_encoders.emplace_back();
    encoder.construct<EncodingType::kTwoDoubleConstant>(v1.value, value2, v1.count);
    chunk.encoding_state = EncodingState{.encoding_type = EncodingType::kTwoDoubleConstant, .has_last_stalenan = false};
    chunk.encoder.external_index = storage_.variant_encoders.index_of(encoder);
  }

  PROMPP_ALWAYS_INLINE void switch_to_asc_integer(chunk::DataChunk& chunk,
                                                  const encoder::value::ConstantValue& v1,
                                                  const encoder::value::ConstantValue& v2,
                                                  const encoder::value::ConstantValue& v3) const {
    auto& encoder = storage_.variant_encoders.emplace_back();
    encoder.construct<EncodingType::kAscInteger>(v1, v2, v3);
    chunk.encoding_state = EncodingState{.encoding_type = EncodingType::kAscInteger, .has_last_stalenan = false};
    chunk.encoder.external_index = storage_.variant_encoders.index_of(encoder);
  }

  PROMPP_ALWAYS_INLINE void switch_to_values_gorilla(chunk::DataChunk& chunk,
                                                     const encoder::value::ConstantValue& v1,
                                                     const encoder::value::ConstantValue& v2,
                                                     const encoder::value::ConstantValue& v3) const {
    auto& encoder = storage_.variant_encoders.emplace_back();
    encoder.construct<EncodingType::kValuesGorilla>(v1, v2, v3);
    chunk.encoding_state = EncodingState{.encoding_type = EncodingType::kValuesGorilla, .has_last_stalenan = false};
    chunk.encoder.external_index = storage_.variant_encoders.index_of(encoder);
  }

  PROMPP_ALWAYS_INLINE void switch_to_asc_integer_then_values_gorilla(chunk::DataChunk& chunk,
                                                                      encoder::value::AscIntegerEncoder&& asc_int_encoder,
                                                                      double value) const {
    auto& encoder = storage_.variant_encoders.emplace_back();
    encoder.construct<EncodingType::kAscIntegerThenValuesGorilla>(std::move(asc_int_encoder), value);
    chunk.encoding_state = EncodingState{.encoding_type = EncodingType::kAscIntegerThenValuesGorilla, .has_last_stalenan = false};
    chunk.encoder.external_index = storage_.variant_encoders.index_of(encoder);
  }

  PROMPP_ALWAYS_INLINE void switch_to_gorilla(chunk::DataChunk& chunk,
                                              int64_t timestamp,
                                              const encoder::value::ConstantValue& v1,
                                              const encoder::value::ConstantValue& v2,
                                              const encoder::value::ConstantValue& v3) const {
    chunk.timestamp_encoder_state_id = storage_.timestamp_encoder.encode(chunk.timestamp_encoder_state_id, timestamp);
    auto& timestamp_stream = storage_.timestamp_encoder.get_stream(chunk.timestamp_encoder_state_id);
    encoder::timestamp::TimestampDecoder timestamp_decoder(timestamp_stream.reader());

    const auto& encoder = storage_.gorilla_encoders.emplace_back(timestamp_decoder, v1, v2, v3);
    chunk.encoding_state = EncodingState{.encoding_type = EncodingType::kGorilla, .has_last_stalenan = false};
    chunk.encoder.external_index = storage_.gorilla_encoders.index_of(encoder);
  }
};

}  // namespace series_data