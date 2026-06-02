#pragma once

#include "traits.h"

namespace series_data::decoder {

class ConstantDecodeIterator : public DecodeIteratorTrait<ConstantDecodeIterator> {
 public:
  template <class BitSequenceWithItemsCount>
  ConstantDecodeIterator(const BitSequenceWithItemsCount& timestamp_stream, double value, bool is_last_stalenan)
      : ConstantDecodeIterator(timestamp_stream.count(), timestamp_stream.reader(), value, is_last_stalenan) {}
  constexpr ConstantDecodeIterator(uint8_t samples_count, const BareBones::BitSequenceReader& timestamp_reader, double value, bool is_last_stalenan)
      : data_{
            .sample = {.value = value},
            .remaining_samples = samples_count,
            .last_stalenan = is_last_stalenan,
            .timestamp_decoder{timestamp_reader},
        } {
    if (data_.remaining_samples > 0) [[likely]] {
      data_.sample.timestamp = data_.timestamp_decoder.decode();
    }
  }

  PROMPP_ALWAYS_INLINE ConstantDecodeIterator& operator++() noexcept {
    decode();
    update_sample();
    return *this;
  }

  PROMPP_ALWAYS_INLINE ConstantDecodeIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double decoded_value() const noexcept {
    if (data_.remaining_samples == 1 && data_.last_stalenan) [[unlikely]] {
      return BareBones::Encoding::Gorilla::STALE_NAN;
    }

    return data_.sample.value;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE PromPP::Primitives::Timestamp decoded_timestamp() const noexcept { return data_.timestamp_decoder.timestamp(); }

 protected:
  friend DecodeIteratorTrait;

  struct Data {
    encoder::Sample sample{};
    uint8_t remaining_samples{};
    bool last_stalenan{false};
    encoder::timestamp::TimestampDecoder timestamp_decoder;
  };

  static_assert(DecodeIteratorDataWithTimestampDecoder<Data>);

  Data data_;

  PROMPP_ALWAYS_INLINE bool decode() noexcept { return decode_timestamp(data_); }

  PROMPP_ALWAYS_INLINE void update_sample() noexcept {
    data_.sample.timestamp = decoded_timestamp();
    data_.sample.value = decoded_value();
  }
};

}  // namespace series_data::decoder
