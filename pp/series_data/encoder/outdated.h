#pragma once

#include "bit_sequence.h"
#include "timestamp/encoder.h"

namespace series_data::encoder {

template <BareBones::ReallocatorInterface Reallocator>
class PROMPP_ATTRIBUTE_PACKED OutdatedEncoder {
 public:
  PROMPP_ALWAYS_INLINE OutdatedEncoder(int64_t timestamp, double value) : count_{1} {
    timestamp_encoder_.encode(timestamp, stream_);
    values_encoder_.encode_first(value, stream_);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double last_value() const noexcept { return values_encoder_.value(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE int64_t timestamp() const noexcept { return timestamp_encoder_.timestamp(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t samples_count() const noexcept { return count_; }

  PROMPP_ALWAYS_INLINE void encode(int64_t timestamp, double value) {
    if (count_ == 1) [[unlikely]] {
      timestamp_encoder_.encode_delta(timestamp, stream_);
    } else {
      timestamp_encoder_.encode_delta_of_delta(timestamp, stream_);
    }
    values_encoder_.encode(value, stream_);
    ++count_;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return stream_.allocated_memory(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const CompactBitSequence<Reallocator>& stream() const noexcept { return stream_; }

 private:
  using TimestampEncoder = BareBones::Encoding::Gorilla::ZigZagTimestampEncoder<>;
  using ValuesEncoder = BareBones::Encoding::Gorilla::ValuesEncoder;

  TimestampEncoder timestamp_encoder_;
  ValuesEncoder values_encoder_;
  CompactBitSequence<Reallocator> stream_;
  uint32_t count_{};
};

}  // namespace series_data::encoder

template <BareBones::ReallocatorInterface Reallocator>
struct BareBones::IsTriviallyReallocatable<series_data::encoder::OutdatedEncoder<Reallocator>> : std::true_type {};
