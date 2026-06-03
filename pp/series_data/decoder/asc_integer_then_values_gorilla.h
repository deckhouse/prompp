#pragma once

#include "asc_integer.h"
#include "values_gorilla.h"

namespace series_data::decoder {

class AscIntegerThenValuesGorillaDecodeIterator : public DecodeIteratorTrait<AscIntegerThenValuesGorillaDecodeIterator> {
 public:
  template <class BitSequenceWithItemsCount>
  AscIntegerThenValuesGorillaDecodeIterator(const BitSequenceWithItemsCount& timestamp_stream, const BareBones::BitSequenceReader& reader)
      : AscIntegerThenValuesGorillaDecodeIterator(timestamp_stream.count(), timestamp_stream.reader(), reader) {}
  AscIntegerThenValuesGorillaDecodeIterator(uint8_t samples_count,
                                            const BareBones::BitSequenceReader& timestamp_reader,
                                            const BareBones::BitSequenceReader& values_reader)
      : data_{
            .remaining_samples = samples_count,
            .asc_integer{},
            .timestamp_decoder{timestamp_reader},
            .reader{values_reader},
        } {
    if (data_.remaining_samples > 0) [[likely]] {
      decode_timestamp();
      decode_value();
      update_sample();
    }
  }

  PROMPP_ALWAYS_INLINE AscIntegerThenValuesGorillaDecodeIterator& operator++() noexcept {
    if (decode()) [[likely]] {
      update_sample();
    }
    return *this;
  }

  PROMPP_ALWAYS_INLINE AscIntegerThenValuesGorillaDecodeIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE double decoded_value() const noexcept {
    if (data_.value_type == encoder::ValueType::kStaleNan) [[unlikely]] {
      return BareBones::Encoding::Gorilla::STALE_NAN;
    }

    if (data_.value_type == encoder::ValueType::kSwitchToValuesGorillaMark) [[unlikely]] {
      return data_.values_gorilla.decoder.value();
    }

    return static_cast<double>(data_.asc_integer.decoder.timestamp());
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE PromPP::Primitives::Timestamp decoded_timestamp() const noexcept { return data_.timestamp_decoder.timestamp(); }

 protected:
  struct Data {
    struct AscIntegerState {
      AscIntegerDecodeIterator::Decoder decoder;
      BareBones::Encoding::Gorilla::GorillaState gorilla_state{BareBones::Encoding::Gorilla::GorillaState::kFirstPoint};
    };

    struct ValuesGorillaState {
      ValuesGorillaDecodeIterator::Decoder decoder;
    };

    encoder::Sample sample{};
    uint8_t remaining_samples{};
    encoder::ValueType value_type{encoder::ValueType::kValue};
    union {
      AscIntegerState asc_integer;
      ValuesGorillaState values_gorilla;
    };
    encoder::timestamp::TimestampDecoder timestamp_decoder;
    BareBones::BitSequenceReader reader;
  };

  static_assert(DecodeIteratorDataWithTimestampDecoder<Data>);

  Data data_;

 private:
  friend DecodeIteratorTrait;

  PROMPP_ALWAYS_INLINE bool decode() noexcept {
    if (decoder::decode_timestamp(data_)) [[likely]] {
      decode_value();
      return true;
    }

    return false;
  }

  PROMPP_ALWAYS_INLINE void update_sample() noexcept {
    data_.sample.timestamp = decoded_timestamp();
    data_.sample.value = decoded_value();
  }

  PROMPP_ALWAYS_INLINE void decode_timestamp() noexcept { std::ignore = data_.timestamp_decoder.decode(); }

  PROMPP_ALWAYS_INLINE void decode_value() noexcept {
    using enum encoder::ValueType;

    if (data_.value_type != kSwitchToValuesGorillaMark) {
      if (data_.value_type = data_.asc_integer.decoder.decode(data_.reader, data_.asc_integer.gorilla_state); data_.value_type == kSwitchToValuesGorillaMark)
          [[unlikely]] {
        switch_to_values_gorilla();
      }
    } else {
      ValuesGorillaDecodeIterator::decode_value<ValuesGorillaDecodeIterator::SampleType::kOther>(data_.values_gorilla.decoder, data_.reader);
    }
  }

  PROMPP_ALWAYS_INLINE void switch_to_values_gorilla() noexcept {
    std::construct_at(&data_.values_gorilla);
    ValuesGorillaDecodeIterator::decode_value<ValuesGorillaDecodeIterator::SampleType::kFirst>(data_.values_gorilla.decoder, data_.reader);
  }
};

}  // namespace series_data::decoder
