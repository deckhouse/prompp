#pragma once

#include "series_data/encoder/value/two_double_constant.h"
#include "traits.h"

namespace series_data::decoder {

class TwoDoubleConstantDecodeIterator : public DecodeIteratorTrait<TwoDoubleConstantDecodeIterator> {
 public:
  template <class BitSequenceWithItemsCount>
  TwoDoubleConstantDecodeIterator(const BitSequenceWithItemsCount& timestamp_stream,
                                  const encoder::value::TwoDoubleConstantEncoder& encoder,
                                  bool is_last_stalenan)
      : TwoDoubleConstantDecodeIterator(timestamp_stream.count(), timestamp_stream.reader(), encoder, is_last_stalenan) {}

  TwoDoubleConstantDecodeIterator(uint8_t samples_count,
                                  const BareBones::BitSequenceReader& timestamp_reader,
                                  const encoder::value::TwoDoubleConstantEncoder& encoder,
                                  bool is_last_stalenan)
      : data_{
            .sample = {.value = encoder.value1()},
            .remaining_samples = samples_count,
            .value1_count = encoder.value1_count(),
            .last_stalenan = is_last_stalenan,
            .value1 = encoder.value1(),
            .value2 = encoder.value2(),
            .timestamp_decoder{timestamp_reader},
        } {
    if (data_.remaining_samples > 0) [[likely]] {
      data_.sample.timestamp = data_.timestamp_decoder.decode();
    }
  }

  PROMPP_ALWAYS_INLINE TwoDoubleConstantDecodeIterator& operator++() noexcept {
    if (decode()) [[likely]] {
      update_sample();
    }
    return *this;
  }

  PROMPP_ALWAYS_INLINE TwoDoubleConstantDecodeIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double decoded_value() const noexcept {
    if (data_.remaining_samples == 1 && data_.last_stalenan) [[unlikely]] {
      return BareBones::Encoding::Gorilla::STALE_NAN;
    }

    return data_.value1_count > 0 ? data_.value1 : data_.value2;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE PromPP::Primitives::Timestamp decoded_timestamp() const noexcept { return data_.timestamp_decoder.timestamp(); }

 protected:
  struct Data {
    encoder::Sample sample{};
    uint8_t remaining_samples{};
    uint8_t value1_count;
    bool last_stalenan{false};
    double value1;
    double value2;
    encoder::timestamp::TimestampDecoder timestamp_decoder;
  };

  static_assert(DecodeIteratorDataWithTimestampDecoder<Data>);

  Data data_;

 private:
  friend DecodeIteratorTrait;

  PROMPP_ALWAYS_INLINE bool decode() noexcept {
    if (data_.value1_count > 0) [[likely]] {
      --data_.value1_count;
    }

    return decode_timestamp(data_);
  }

  PROMPP_ALWAYS_INLINE void update_sample() noexcept {
    data_.sample.timestamp = decoded_timestamp();
    data_.sample.value = decoded_value();
  }
};

}  // namespace series_data::decoder
