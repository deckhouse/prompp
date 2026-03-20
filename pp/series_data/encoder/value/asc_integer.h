#pragma once

#include "constant_value.h"
#include "series_data/common.h"
#include "series_data/encoder/bit_sequence.h"
#include "series_data/encoder/numeric.h"
#include "series_data/encoder/zig_zag_timestamp_gorilla.h"

namespace series_data::encoder::value {

template <BareBones::ReallocatorInterface Reallocator>
class PROMPP_ATTRIBUTE_PACKED AscIntegerEncoder {
 private:
  using CompactBitSequence = series_data::encoder::CompactBitSequence<Reallocator>;

 public:
  using EncoderDeltaType = int32_t;
  using Encoder = ZigZagTimestampEncoder<EncoderDeltaType>;

  PROMPP_ALWAYS_INLINE explicit AscIntegerEncoder(double value) { encoder_.encode(static_cast<int64_t>(value), stream_); }

  PROMPP_ALWAYS_INLINE AscIntegerEncoder(const ConstantValue& v1, const ConstantValue& v2, const ConstantValue& v3) {
    encoder_.encode(static_cast<int64_t>(v1.value), stream_);

    encode_multiple(v1, v2, v3);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static bool can_be_encoded(double value1, uint8_t value1_count, double value2, double value3) {
    if (!is_int(value1)) {
      return false;
    }

    if (value1_count > 1) {
      if (BareBones::Encoding::Gorilla::isstalenan(value2)) [[unlikely]] {
        return is_int_and_ge_than(value3, value1);
      }
    }

    return is_int_and_ge_than(value2, value1) && (is_int_and_ge_than(value3, value2) || BareBones::Encoding::Gorilla::isstalenan(value3));
  }

  PROMPP_ALWAYS_INLINE void encode_second(double value) { encoder_.encode_delta(static_cast<int64_t>(value), stream_); }

  PROMPP_ALWAYS_INLINE bool encode(EncodingState& state, double value) noexcept {
    state.has_last_stalenan = BareBones::Encoding::Gorilla::isstalenan(value);
    return encode(value);
  }

  PROMPP_ALWAYS_INLINE bool operator==(const AscIntegerEncoder& other) const noexcept { return stream_ == other.stream_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return stream_.allocated_memory(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_actual(const EncodingState& state, double value) const noexcept {
    return is_values_strictly_equal(value, last_value(state));
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double last_value(const EncodingState& state) const noexcept {
    if (state.has_last_stalenan) [[unlikely]] {
      return BareBones::Encoding::Gorilla::STALE_NAN;
    }
    return static_cast<double>(encoder_.timestamp());
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const CompactBitSequence& stream() const noexcept { return stream_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE CompactBitSequence& stream() noexcept { return stream_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE CompactBitSequence release_stream() && noexcept { return std::move(stream_); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE CompactBitSequence finalize_stream() noexcept {
    auto stream = std::move(stream_);
    stream.shrink_to_fit();
    return stream;
  }

 private:
  Encoder encoder_;
  CompactBitSequence stream_;

  PROMPP_ALWAYS_INLINE bool encode(double value) noexcept {
    if (!BareBones::Encoding::Gorilla::isstalenan(value)) [[likely]] {
      if (!is_int_and_ge_than(value, static_cast<double>(encoder_.timestamp()))) [[unlikely]] {
        return false;
      }
    }

    encoder_.encode_delta_of_delta_with_stale_nan(value, stream_);
    return true;
  }

  PROMPP_ALWAYS_INLINE void encode_multiple(const ConstantValue& v1, ConstantValue v2, const ConstantValue& v3) {
    if (v1.count > 1) {
      encode_second(v1.value);
      for (uint8_t i = 2; i < v1.count; ++i) {
        encode(v1.value);
      }
    } else {
      encode_second(v2.value);
      --v2.count;
    }

    for (uint8_t i = 0; i < v2.count; ++i) {
      encode(v2.value);
    }

    for (uint8_t i = 0; i < v3.count; ++i) {
      encode(v3.value);
    }
  }

  PROMPP_ALWAYS_INLINE static bool is_int_and_ge_than(double value2, double value1) noexcept {
    return is_int(value2) && is_in_bounds(static_cast<int64_t>(value2) - static_cast<int64_t>(value1), 0, std::numeric_limits<EncoderDeltaType>::max());
  }
};

}  // namespace series_data::encoder::value

template <BareBones::ReallocatorInterface Reallocator>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::value::AscIntegerEncoder<Reallocator>> : std::true_type {};
