#pragma once

#include "bare_bones/gorilla.h"
#include "bit_sequence.h"
#include "sample.h"
#include "series_data/common.h"
#include "timestamp/encoder.h"
#include "value/constant_value.h"

namespace series_data::encoder {

class PROMPP_ATTRIBUTE_PACKED GorillaEncoder {
 public:
  PROMPP_ALWAYS_INLINE GorillaEncoder(int64_t timestamp, double value) {
    timestamp_encoder_.encode(timestamp, stream_.stream);
    values_encoder_.encode_first(value, stream_.stream);
  }

  PROMPP_ALWAYS_INLINE GorillaEncoder(timestamp::TimestampDecoder& timestamp_decoder,
                                      const value::ConstantValue& v1,
                                      const value::ConstantValue& v2,
                                      const value::ConstantValue& v3) {
    timestamp_encoder_.encode(timestamp_decoder.decode(), stream_.stream);
    values_encoder_.encode_first(v1.value, stream_.stream);

    encode_multiple(timestamp_decoder, v1, v2, v3);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_actual(const EncodingState& state, double value) const noexcept {
    return std::bit_cast<uint64_t>(last_value(state)) == std::bit_cast<uint64_t>(value);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double last_value(const EncodingState& state) const noexcept {
    if (state.has_last_stalenan) [[unlikely]] {
      return BareBones::Encoding::Gorilla::STALE_NAN;
    }
    return values_encoder_.value();
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t timestamp() const noexcept { return timestamp_encoder_.timestamp(); }

  PROMPP_ALWAYS_INLINE uint8_t encode(EncodingState& state, int64_t timestamp, double value) {
    state.has_last_stalenan = BareBones::Encoding::Gorilla::isstalenan(value);
    return encode(timestamp, value);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE CompactBitSequence finalize_stream() noexcept {
    auto stream = std::move(stream_.stream);
    stream.shrink_to_fit();
    return stream;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return stream_.allocated_memory(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const BitSequenceWithItemsCount& stream() const noexcept { return stream_; }

 private:
  using TimestampEncoder = BareBones::Encoding::Gorilla::ZigZagTimestampEncoder<>;
  using ValuesEncoder = BareBones::Encoding::Gorilla::ValuesEncoder;

  TimestampEncoder timestamp_encoder_;
  ValuesEncoder values_encoder_;
  BitSequenceWithItemsCount stream_;

  PROMPP_ALWAYS_INLINE uint8_t encode(int64_t timestamp, double value) {
    const auto count = stream_.inc_count();

    if (count == 1) [[unlikely]] {
      timestamp_encoder_.encode_delta(timestamp, stream_.stream);
      values_encoder_.encode(value, stream_.stream);
    } else {
      timestamp_encoder_.encode_delta_of_delta(timestamp, stream_.stream);
      values_encoder_.encode(value, stream_.stream);
    }

    return count + 1;
  }

  PROMPP_ALWAYS_INLINE void encode_multiple(timestamp::TimestampDecoder& timestamp_decoder,
                                            const value::ConstantValue& v1,
                                            const value::ConstantValue& v2,
                                            const value::ConstantValue& v3) {
    for (uint8_t i = 1; i < v1.count; ++i) {
      encode(timestamp_decoder.decode(), v1.value);
    }

    for (uint8_t i = 0; i < v2.count; ++i) {
      encode(timestamp_decoder.decode(), v2.value);
    }

    if (v3.has_value()) {
      encode(timestamp_decoder.decode(), v3.value);
    }
  }
};

}  // namespace series_data::encoder

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::GorillaEncoder> : std::true_type {};
