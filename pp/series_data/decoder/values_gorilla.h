#pragma once

#include "series_data/encoder/value/values_gorilla.h"
#include "traits.h"

namespace series_data::decoder {

class ValuesGorillaDecodeIterator : public DecodeIteratorTrait<ValuesGorillaDecodeIterator> {
 public:
  using Decoder = BareBones::Encoding::Gorilla::ValuesDecoder;

  enum class SampleType : uint8_t {
    kFirst = 0,
    kOther,
  };

  template <class BitSequenceWithItemsCount>
  ValuesGorillaDecodeIterator(const BitSequenceWithItemsCount& timestamp_stream, const BareBones::BitSequenceReader& reader)
      : ValuesGorillaDecodeIterator(timestamp_stream.count(), timestamp_stream.reader(), reader) {}
  ValuesGorillaDecodeIterator(uint8_t samples_count, const BareBones::BitSequenceReader& timestamp_reader, const BareBones::BitSequenceReader& values_reader)
      : data_{
            .remaining_samples = samples_count,
            .timestamp_decoder{timestamp_reader},
            .reader{values_reader},
        } {
    if (data_.remaining_samples > 0) [[likely]] {
      decode_timestamp();
      decode_value<SampleType::kFirst>();
      update_sample();
    }
  }

  PROMPP_ALWAYS_INLINE ValuesGorillaDecodeIterator& operator++() noexcept {
    if (decode()) [[likely]] {
      update_sample();
    }
    return *this;
  }

  PROMPP_ALWAYS_INLINE ValuesGorillaDecodeIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

  template <SampleType Type>
  PROMPP_ALWAYS_INLINE static void decode_value(Decoder& decoder, BareBones::BitSequenceReader& reader) noexcept {
    if constexpr (Type == SampleType::kFirst) {
      decoder.decode_first(reader);
    } else {
      decoder.decode(reader);
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double decoded_value() const noexcept { return data_.decoder.value(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE PromPP::Primitives::Timestamp decoded_timestamp() const noexcept { return data_.timestamp_decoder.timestamp(); }

 protected:
  struct Data {
    encoder::Sample sample{};
    uint8_t remaining_samples{};
    Decoder decoder{};
    encoder::timestamp::TimestampDecoder timestamp_decoder;
    BareBones::BitSequenceReader reader;
  };

  static_assert(DecodeIteratorDataWithTimestampDecoder<Data>);

  Data data_;

 private:
  friend DecodeIteratorTrait;

  PROMPP_ALWAYS_INLINE bool decode() noexcept {
    if (decoder::decode_timestamp(data_)) [[likely]] {
      decode_value<SampleType::kOther>();
      return true;
    }

    return false;
  }

  PROMPP_ALWAYS_INLINE void update_sample() noexcept {
    data_.sample.timestamp = decoded_timestamp();
    data_.sample.value = decoded_value();
  }

  PROMPP_ALWAYS_INLINE void decode_timestamp() noexcept { std::ignore = data_.timestamp_decoder.decode(); }

  template <SampleType Type>
  PROMPP_ALWAYS_INLINE void decode_value() noexcept {
    decode_value<Type>(data_.decoder, data_.reader);
  }
};

}  // namespace series_data::decoder
