#pragma once

#include <bit>
#include <cmath>
#include <cstdint>

#include "primitives/sample.h"
#include "prometheus/value.h"

namespace PromPP::WAL::hashdex::scraper::encoding {
enum class SampleValueType : uint8_t { kUint32 = 0b0000'0000, kDouble, kUint8, kUint16, kFloat, kZero, kNaN };

[[nodiscard]] PROMPP_LAMBDA_INLINE inline SampleValueType value_type(const double val) noexcept {
  if (std::isnan(val)) [[unlikely]] {
    return SampleValueType::kNaN;
  }
  if (val == 0.0) [[unlikely]] {
    return SampleValueType::kZero;
  }

  if (std::trunc(val) == val && val > 0.0) [[likely]] {
    const auto uval = static_cast<uint64_t>(val);
    if (uval <= std::numeric_limits<uint8_t>::max()) {
      return SampleValueType::kUint8;
    }
    if (uval <= std::numeric_limits<uint16_t>::max()) {
      return SampleValueType::kUint16;
    }
    if (uval <= std::numeric_limits<uint32_t>::max()) {
      return SampleValueType::kUint32;
    }
  }

  if (const auto f = static_cast<float>(val); static_cast<double>(f) == val) [[unlikely]] {
    return SampleValueType::kFloat;
  }

  return SampleValueType::kDouble;
}

struct LayoutMarker {
  uint8_t raw;

  [[nodiscard]] PROMPP_LAMBDA_INLINE bool has_timestamp() const noexcept { return raw & 0b1000'0000; }

  [[nodiscard]] PROMPP_LAMBDA_INLINE uint8_t count_size_in_bytes() const noexcept { return (raw >> 4) & 0b0000'0111; }

  [[nodiscard]] PROMPP_LAMBDA_INLINE SampleValueType value_type() const noexcept { return static_cast<SampleValueType>(raw & 0b0000'1111); }

  static PROMPP_LAMBDA_INLINE LayoutMarker make(bool has_ts, uint32_t labels_count, SampleValueType value_type) noexcept {
    const uint8_t bytes_for_count = (std::bit_width(labels_count) + 7) / 8;

    return {static_cast<uint8_t>((has_ts ? 0b1000'0000 : 0) | (bytes_for_count << 4) | (static_cast<uint8_t>(value_type) & 0b0000'1111))};
  }
};

class SampleCodec {
 public:
  static PROMPP_LAMBDA_INLINE char* encode(char* out, const LayoutMarker layout, Primitives::Sample sample) {
    using encoding::SampleValueType;

    const double val = sample.value();
    if (const auto type = layout.value_type(); type == SampleValueType::kUint32) [[likely]] {
      out = write_value(out, static_cast<uint32_t>(val));
    } else if (type == SampleValueType::kDouble) [[likely]] {
      out = write_value(out, val);
    } else if (type == SampleValueType::kUint8) [[unlikely]] {
      out = write_value(out, static_cast<uint8_t>(val));
    } else if (type == SampleValueType::kUint16) [[unlikely]] {
      out = write_value(out, static_cast<uint16_t>(val));
    } else if (type == SampleValueType::kFloat) [[unlikely]] {
      out = write_value(out, static_cast<float>(val));
    }

    if (layout.has_timestamp()) [[unlikely]] {
      out = write_value(out, sample.timestamp());
    }

    return out;
  }

  struct DecodeResult {
    const char* next;
    Primitives::Sample sample;
  };

  static PROMPP_LAMBDA_INLINE DecodeResult decode(const char* in, const LayoutMarker layout, int64_t default_ts) {
    using encoding::SampleValueType;

    DecodeResult result{};

    double& val = result.sample.value();
    uint64_t chunk;
    std::memcpy(&chunk, in, sizeof(chunk));

    if (const auto type = layout.value_type(); type == SampleValueType::kUint32) [[likely]] {
      val = static_cast<double>(static_cast<uint32_t>(chunk));
      in += sizeof(uint32_t);
    } else if (type == SampleValueType::kDouble) [[likely]] {
      val = std::bit_cast<double>(chunk);
      in += sizeof(double);
    } else if (type == SampleValueType::kUint8) [[unlikely]] {
      val = static_cast<double>(static_cast<uint8_t>(chunk));
      in += sizeof(uint8_t);
    } else if (type == SampleValueType::kUint16) [[unlikely]] {
      val = static_cast<double>(static_cast<uint16_t>(chunk));
      in += sizeof(uint16_t);
    } else if (type == SampleValueType::kFloat) [[unlikely]] {
      val = static_cast<double>(std::bit_cast<float>(static_cast<uint32_t>(chunk)));
      in += sizeof(float);
    } else if (type == SampleValueType::kZero) [[unlikely]] {
      val = 0.0;
    } else {
      val = Prometheus::kNormalNan;
    }

    if (auto& timestamp = result.sample.timestamp(); layout.has_timestamp()) [[unlikely]] {
      std::memcpy(&timestamp, in, sizeof(timestamp));
      in += sizeof(timestamp);
    } else {
      timestamp = default_ts;
    }

    result.next = in;

    return result;
  }

 private:
  template <typename T>
  PROMPP_ALWAYS_INLINE static char* write_value(char* out, const T& val) noexcept {
    std::memcpy(out, &val, sizeof(T));
    return out + sizeof(T);
  }
};

}  // namespace PromPP::WAL::hashdex::scraper::encoding