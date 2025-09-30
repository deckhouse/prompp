#pragma once

#include "bare_bones/gorilla.h"
#include "constant_value.h"
#include "series_data/common.h"
#include "series_data/encoder/bit_sequence.h"
#include "series_data/encoder/numeric.h"

namespace series_data::encoder::value {

class PROMPP_ATTRIBUTE_PACKED ValuesGorillaEncoder {
 public:
  PROMPP_ALWAYS_INLINE explicit ValuesGorillaEncoder(double value, uint32_t count) { values_encoder_.encode_first(value, count, stream_); }
  PROMPP_ALWAYS_INLINE ValuesGorillaEncoder(const ConstantValue& v1, const ConstantValue& v2, const ConstantValue& v3) {
    values_encoder_.encode_first(v1.value, v1.count, stream_);

    encode_multiple(v2, v3);
  }
  PROMPP_ALWAYS_INLINE ValuesGorillaEncoder(CompactBitSequence&& stream, double value) : stream_(std::move(stream)) {
    values_encoder_.encode_first(value, stream_);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_actual(const EncodingState& state, double value) const noexcept {
    return is_values_strictly_equal(last_value(state), value);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double last_value(const EncodingState& state) const noexcept {
    if (state.has_last_stalenan) [[unlikely]] {
      return BareBones::Encoding::Gorilla::STALE_NAN;
    }
    return values_encoder_.value();
  }

  PROMPP_ALWAYS_INLINE void encode(EncodingState& state, double value) {
    state.has_last_stalenan = BareBones::Encoding::Gorilla::isstalenan(value);
    encode(value);
  }

  PROMPP_ALWAYS_INLINE void encode(double value, uint32_t count) { values_encoder_.encode(value, count, stream_); }

  bool operator==(const ValuesGorillaEncoder& other) const noexcept { return stream_ == other.stream_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return stream_.allocated_memory(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const CompactBitSequence& stream() const noexcept { return stream_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE CompactBitSequence& stream() noexcept { return stream_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE CompactBitSequence finalize_stream() noexcept {
    auto stream = std::move(stream_);
    stream.shrink_to_fit();
    return stream;
  }

 protected:
  using Encoder = BareBones::Encoding::Gorilla::ValuesEncoder;

  Encoder values_encoder_;
  CompactBitSequence stream_;

  PROMPP_ALWAYS_INLINE void encode(double value) { values_encoder_.encode(value, stream_); }

  PROMPP_ALWAYS_INLINE void encode_multiple(const ConstantValue& v2, const ConstantValue& v3) {
    values_encoder_.encode(v2.value, v2.count, stream_);

    if (v3.has_value()) {
      encode(v3.value);
    }
  }
};

}  // namespace series_data::encoder::value

template <>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::value::ValuesGorillaEncoder> : std::true_type {};
