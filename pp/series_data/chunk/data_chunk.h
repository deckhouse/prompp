#pragma once

#include "bare_bones/preprocess.h"
#include "series_data/common.h"
#include "series_data/encoder/timestamp/state.h"
#include "series_data/encoder/value/float32_constant.h"
#include "series_data/encoder/value/uint32_constant.h"

namespace series_data::chunk {

struct PROMPP_ATTRIBUTE_PACKED DataChunk {
  enum class Type : uint8_t {
    kOpen = 0,
    kFinalized,
  };

  union PROMPP_ATTRIBUTE_PACKED EncoderData {
    encoder::value::Uint32ConstantEncoder uint32_constant;
    encoder::value::Float32ConstantEncoder float32_constant;
    uint32_t external_index;

    PROMPP_ALWAYS_INLINE bool operator==(const EncoderData& other) const noexcept { return external_index == other.external_index; }
  };

  EncoderData encoder{.external_index = 0};
  encoder::timestamp::StateId timestamp_encoder_state_id{encoder::timestamp::kInvalidStateId};
  EncodingState encoding_state{.encoding_type = EncodingType::kUnknown, .has_last_stalenan = false};

  DataChunk() = default;
  DataChunk(const DataChunk&) noexcept = default;

  DataChunk(uint32_t encoder_id, encoder::timestamp::StateId _timestamp_encoder_state_id, EncodingState _encoding_state)
      : encoder{.external_index = encoder_id}, timestamp_encoder_state_id(_timestamp_encoder_state_id), encoding_state(_encoding_state) {}

  DataChunk& operator=(const DataChunk& other) noexcept {
    if (this != &other) {
      encoder.external_index = other.encoder.external_index;
      timestamp_encoder_state_id = other.timestamp_encoder_state_id;
      encoding_state = other.encoding_state;
    }

    return *this;
  }

  bool operator==(const DataChunk&) const noexcept = default;

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_empty() const noexcept { return encoding_state.encoding_type == EncodingType::kUnknown; }

  PROMPP_ALWAYS_INLINE void reset() noexcept {
    encoder.external_index = 0;
    timestamp_encoder_state_id = encoder::timestamp::kInvalidStateId;
    encoding_state = EncodingState{.encoding_type = EncodingType::kUnknown, .has_last_stalenan = false};
  }

  PROMPP_ALWAYS_INLINE void mark_last_stalenan() noexcept { encoding_state.has_last_stalenan = true; }
  PROMPP_ALWAYS_INLINE void unmark_last_stalenan() noexcept { encoding_state.has_last_stalenan = false; }
};

}  // namespace series_data::chunk

template <>
struct BareBones::IsTriviallyReallocatable<series_data::chunk::DataChunk> : std::true_type {};