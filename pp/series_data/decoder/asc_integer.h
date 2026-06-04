#pragma once

#include "series_data/encoder/value/asc_integer.h"
#include "traits.h"

namespace series_data::decoder {

class AscIntegerDecodeIterator : public DecodeIteratorTrait<AscIntegerDecodeIterator> {
 public:
  using Decoder = encoder::ZigZagTimestampDecoder;

  template <class BitSequenceWithItemsCount>
  AscIntegerDecodeIterator(const BitSequenceWithItemsCount& timestamp_stream, const BareBones::BitSequenceReader& reader)
      : AscIntegerDecodeIterator(timestamp_stream.count(), timestamp_stream.reader(), reader) {}
  AscIntegerDecodeIterator(uint8_t samples_count, const BareBones::BitSequenceReader& timestamp_reader, const BareBones::BitSequenceReader& values_reader)
      : data_{
            .remaining_samples = samples_count,
            .timestamp_decoder{timestamp_reader},
            .reader{values_reader},
        } {
    if (data_.remaining_samples > 0) [[likely]] {
      decode_timestamp();
      decode_value();
      update_sample();
    }
  }

  PROMPP_ALWAYS_INLINE AscIntegerDecodeIterator& operator++() noexcept {
    if (decode()) [[likely]] {
      update_sample();
    }
    return *this;
  }

  PROMPP_ALWAYS_INLINE AscIntegerDecodeIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double decoded_value() const noexcept {
    if (data_.value_type == encoder::ValueType::kStaleNan) [[unlikely]] {
      return BareBones::Encoding::Gorilla::STALE_NAN;
    }

    return static_cast<double>(data_.decoder.timestamp());
  }

 protected:
  struct Data {
    using GorillaState = BareBones::Encoding::Gorilla::GorillaState;

    encoder::Sample sample{};
    uint8_t remaining_samples{};
    GorillaState gorilla_state{GorillaState::kFirstPoint};
    encoder::ValueType value_type{encoder::ValueType::kValue};
    Decoder decoder{};
    encoder::timestamp::TimestampDecoder timestamp_decoder;
    BareBones::BitSequenceReader reader;
  };

  static_assert(DecodeIteratorDataWithTimestampDecoder<Data>);

  Data data_;

 private:
  friend DecodeIteratorTrait;

  PROMPP_ALWAYS_INLINE bool decode() noexcept {
    if (try_decode_timestamp()) [[likely]] {
      decode_value();
      return true;
    }

    return false;
  }

  PROMPP_ALWAYS_INLINE void decode_value() noexcept { data_.value_type = data_.decoder.decode(data_.reader, data_.gorilla_state); }

  PROMPP_ALWAYS_INLINE void update_sample() noexcept {
    data_.sample.timestamp = decoded_timestamp();
    data_.sample.value = decoded_value();
  }
};

}  // namespace series_data::decoder
