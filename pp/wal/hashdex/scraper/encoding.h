#pragma once

#include <bit>
#include <cmath>

#include "bare_bones/bit.h"
#include "primitives/sample.h"
#include "prometheus/value.h"

namespace PromPP::WAL::hashdex::scraper::encoding {
enum class SampleValueType : uint8_t { kUint32 = 0b0000'0000, kDouble, kUint8, kUint16, kFloat, kZero, kNaN };

struct LayoutMarker {
  bool has_ts : 1;
  uint8_t length_bytes : 3;
  SampleValueType sample_value_type : 4;

  [[nodiscard]] PROMPP_LAMBDA_INLINE bool has_timestamp() const noexcept { return has_ts; }

  [[nodiscard]] PROMPP_LAMBDA_INLINE uint8_t size_length_in_bytes() const noexcept { return length_bytes; }

  [[nodiscard]] PROMPP_LAMBDA_INLINE SampleValueType value_type() const noexcept { return sample_value_type; }

  static PROMPP_LAMBDA_INLINE LayoutMarker make(bool has_ts, uint32_t labels_count, SampleValueType value_type) noexcept {
    const uint8_t bytes_for_count = BareBones::Bit::to_ceil_bytes(std::bit_width(labels_count));
    return LayoutMarker{.has_ts = has_ts, .length_bytes = bytes_for_count, .sample_value_type = value_type};
  }
};

class SampleCodec {
 public:
  static constexpr size_t kMaximumEncodingSize = sizeof(Primitives::Sample);

  static char* encode(char* out, const LayoutMarker layout, Primitives::Sample sample) {
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

  static DecodeResult decode(const char* in, const LayoutMarker layout, int64_t default_ts) {
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

  [[nodiscard]] static SampleValueType value_type(const double val) noexcept {
    if (std::isnan(val)) [[unlikely]] {
      return SampleValueType::kNaN;
    }

    if (val == 0.0) [[unlikely]] {
      return SampleValueType::kZero;
    }

    if (std::trunc(val) == val && val > 0.0 && val <= std::numeric_limits<uint32_t>::max()) [[likely]] {
      const auto uval = static_cast<uint64_t>(val);
      if (uval <= std::numeric_limits<uint8_t>::max()) {
        return SampleValueType::kUint8;
      }
      if (uval <= std::numeric_limits<uint16_t>::max()) {
        return SampleValueType::kUint16;
      }
      return SampleValueType::kUint32;
    }

    if (const auto f = static_cast<float>(val); static_cast<double>(f) == val) [[unlikely]] {
      return SampleValueType::kFloat;
    }

    return SampleValueType::kDouble;
  }

 private:
  template <typename T>
  PROMPP_ALWAYS_INLINE static char* write_value(char* out, const T& val) noexcept {
    std::memcpy(out, &val, sizeof(T));
    return out + sizeof(T);
  }
};

class LabelCodec {
 public:
  static constexpr size_t kMaximumEncodingSize = sizeof(uint8_t) + 4 * sizeof(uint32_t);

  static char* encode(char* out,
                      const uint32_t label_name_offset,
                      const uint32_t label_name_length,
                      const uint32_t label_value_offset,
                      const uint32_t label_value_length) noexcept {
    if (label_name_offset == 0 && label_name_length == 0) [[likely]] {
      return encode_value_only(out, label_value_offset, label_value_length);
    }

    if ((label_name_offset | label_name_length | label_value_offset | label_value_length) <= 0xFF) [[likely]] {
      return encode_4_bytes(out, label_name_offset, label_name_length, label_value_offset, label_value_length);
    }

    return encode_generic(out, label_name_offset, label_name_length, label_value_offset, label_value_length);
  }

  struct DecodeResult {
    const char* next;
    uint32_t label_name_offset;
    uint32_t label_name_length;
    uint32_t label_value_offset;
    uint32_t label_value_length;
  };

  static DecodeResult decode(const char* in) noexcept {
    uint64_t chunk;
    std::memcpy(&chunk, in, sizeof(chunk));
    const auto layout = static_cast<uint8_t>(chunk);

    if (layout == 0b01010101) [[likely]] {
      return decode_4_bytes(in, chunk);
    }

    if ((layout & 0x0F) == 0) [[likely]] {
      return decode_value_only(in, chunk, layout);
    }

    return decode_generic(++in, layout);
  }

 private:
  static PROMPP_ALWAYS_INLINE char* encode_value_only(char* out, const uint32_t label_value_offset, const uint32_t label_value_length) noexcept {
    char* start = out++;

    const uint8_t sz2 = push_and_encode(out, label_value_offset);
    out += szm_[sz2];
    const uint8_t sz3 = push_and_encode(out, label_value_length);
    out += szm_[sz3];

    *start = (sz2 << 4) | (sz3 << 6);

    return out;
  }

  static PROMPP_ALWAYS_INLINE char* encode_4_bytes(char* out,
                                                   const uint8_t label_name_offset,
                                                   const uint8_t label_name_length,
                                                   const uint8_t label_value_offset,
                                                   const uint8_t label_value_length) noexcept {
    const uint64_t chunk = static_cast<uint64_t>(0b01010101) | static_cast<uint64_t>(label_name_offset) << 8 | static_cast<uint64_t>(label_name_length) << 16 |
                           static_cast<uint64_t>(label_value_offset) << 24 | static_cast<uint64_t>(label_value_length) << 32;

    std::memcpy(out, &chunk, sizeof(chunk));
    return out + 5;
  }

  static PROMPP_ALWAYS_INLINE char* encode_generic(char* out,
                                                   const uint32_t label_name_offset,
                                                   const uint32_t label_name_length,
                                                   const uint32_t label_value_offset,
                                                   const uint32_t label_value_length) noexcept {
    char* start = out++;
    const uint8_t sz0 = push_and_encode(out, label_name_offset);
    out += szm_[sz0];
    const uint8_t sz1 = push_and_encode(out, label_name_length);
    out += szm_[sz1];
    const uint8_t sz2 = push_and_encode(out, label_value_offset);
    out += szm_[sz2];
    const uint8_t sz3 = push_and_encode(out, label_value_length);
    out += szm_[sz3];

    *start = (sz0) | (sz1 << 2) | (sz2 << 4) | (sz3 << 6);

    return out;
  }

  static PROMPP_ALWAYS_INLINE DecodeResult decode_4_bytes(const char* in, const uint64_t chunk) noexcept {
    const auto name_off = static_cast<uint8_t>(chunk >> 8);
    const auto name_len = static_cast<uint8_t>(chunk >> 16);
    const auto value_off = static_cast<uint8_t>(chunk >> 24);
    const auto value_len = static_cast<uint8_t>(chunk >> 32);

    return DecodeResult{
        .next = in + 5, .label_name_offset = name_off, .label_name_length = name_len, .label_value_offset = value_off, .label_value_length = value_len};
  }

  static PROMPP_ALWAYS_INLINE DecodeResult decode_value_only(const char* in, uint64_t chunk, const uint8_t layout) noexcept {
    const uint8_t sz2 = (layout >> 4) & 0b11;
    const uint8_t sz3 = (layout >> 6) & 0b11;

    chunk >>= 8;
    size_t used = 1;

    uint32_t value_off = 0;
    uint32_t value_len = 0;

    if (sz2 == 0b01) [[likely]] {
      value_off = static_cast<uint8_t>(chunk);
      used += sizeof(uint8_t);
      chunk >>= BareBones::Bit::to_bits(sizeof(uint8_t));
    } else if (sz2 == 0b10) [[unlikely]] {
      value_off = static_cast<uint16_t>(chunk);
      used += sizeof(uint16_t);
      chunk >>= BareBones::Bit::to_bits(sizeof(uint16_t));
    } else if (sz2 == 0b11) [[unlikely]] {
      value_off = static_cast<uint32_t>(chunk);
      used += sizeof(uint32_t);
      chunk >>= BareBones::Bit::to_bits(sizeof(uint32_t));
    }
    if (sz3 == 0b01) [[likely]] {
      value_len = static_cast<uint8_t>(chunk);
      used += sizeof(uint8_t);
    } else if (sz3 == 0b10) [[unlikely]] {
      value_len = static_cast<uint16_t>(chunk);
      used += sizeof(uint16_t);
    } else if (sz3 == 0b11) [[unlikely]] {
      if (used + sizeof(uint32_t) <= sizeof(chunk)) [[likely]] {
        value_len = static_cast<uint32_t>(chunk);
      } else [[unlikely]] {
        std::memcpy(&value_len, in + used, sizeof(value_len));
      }
      used += sizeof(value_len);
    }

    return DecodeResult{.next = in + used, .label_name_offset = 0, .label_name_length = 0, .label_value_offset = value_off, .label_value_length = value_len};
  }

  static PROMPP_ALWAYS_INLINE DecodeResult decode_generic(const char* in, const uint8_t layout) noexcept {
    const uint8_t sz0 = layout & 0b11;
    const uint8_t sz1 = (layout >> 2) & 0b11;
    const uint8_t sz2 = (layout >> 4) & 0b11;
    const uint8_t sz3 = (layout >> 6) & 0b11;

    const uint32_t name_off = read_val_partial(in, sz0);
    const uint32_t name_len = read_val_partial(in, sz1);
    const uint32_t value_off = read_val_partial(in, sz2);
    const uint32_t value_len = read_val_partial(in, sz3);

    return DecodeResult{
        .next = in, .label_name_offset = name_off, .label_name_length = name_len, .label_value_offset = value_off, .label_value_length = value_len};
  }

  static PROMPP_ALWAYS_INLINE uint8_t push_and_encode(char* out, uint32_t v) noexcept {
    if (v == 0) [[unlikely]] {
      return 0b00;
    }
    std::memcpy(out, &v, sizeof(v));
    if (v <= 0xFF) [[likely]] {
      return 0b01;
    }
    if (v <= 0xFFFF) [[unlikely]] {
      return 0b10;
    }

    return 0b11;
  }

  static PROMPP_ALWAYS_INLINE uint32_t read_val_partial(const char*& p, uint8_t sz) noexcept {
    if (sz == 0b01) [[likely]] {
      return *p++;
    }
    if (sz == 0b00) {
      return 0;
    }
    if (sz == 0b10) {
      uint16_t v;
      std::memcpy(&v, p, 2);
      p += 2;
      return v;
    }
    uint32_t v;
    std::memcpy(&v, p, 4);
    p += 4;
    return v;
  }

  static constexpr uint8_t szm_[4] = {0, 1, 2, 4};
};

class LayoutCountCodec {
 public:
  static constexpr size_t kMaximumEncodingSize = sizeof(uint64_t);

  static PROMPP_ALWAYS_INLINE char* encode(char* out, const LayoutMarker layout, const uint32_t count) noexcept {
    uint64_t chunk = 0;
    std::memcpy(&chunk, &layout, sizeof(layout));
    chunk |= static_cast<uint64_t>(count) << 8;
    std::memcpy(out, &chunk, sizeof(chunk));

    const uint32_t bytes_written = sizeof(layout) + layout.size_length_in_bytes();
    return out + bytes_written;
  }

  struct DecodeResult {
    const char* next;
    LayoutMarker layout;
    uint32_t count;
  };

  static PROMPP_ALWAYS_INLINE DecodeResult decode(const char* in) noexcept {
    uint64_t chunk;
    std::memcpy(&chunk, in, sizeof(chunk));

    LayoutMarker layout{};
    std::memcpy(&layout, &chunk, sizeof(layout));

    chunk >>= 8;
    const uint64_t mask = (1ULL << BareBones::Bit::to_bits(layout.size_length_in_bytes())) - 1;

    auto labels_count = static_cast<uint32_t>(chunk & mask);

    return {in + sizeof(layout) + layout.size_length_in_bytes(), layout, labels_count};
  }
};

static constexpr uint32_t metric_maximum_encoding_size(uint32_t labels_count) noexcept {
  return LayoutCountCodec::kMaximumEncodingSize + LabelCodec::kMaximumEncodingSize * labels_count + SampleCodec::kMaximumEncodingSize;
}

}  // namespace PromPP::WAL::hashdex::scraper::encoding