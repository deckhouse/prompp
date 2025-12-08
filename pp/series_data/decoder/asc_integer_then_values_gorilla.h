#pragma once

#include "asc_integer.h"
#include "values_gorilla.h"

namespace series_data::decoder {

class AscIntegerThenValuesGorillaDecodeIterator : public SeparatedTimestampValueDecodeIteratorTrait<AscIntegerThenValuesGorillaDecodeIterator> {
 public:
  template <class BitSequenceWithItemsCount>
  AscIntegerThenValuesGorillaDecodeIterator(const BitSequenceWithItemsCount& timestamp_stream,
                                            const BareBones::BitSequenceReader& reader,
                                            bool is_last_stalenan)
      : AscIntegerThenValuesGorillaDecodeIterator(timestamp_stream.count(), timestamp_stream.reader(), reader, is_last_stalenan) {}
  AscIntegerThenValuesGorillaDecodeIterator(uint8_t samples_count,
                                            const BareBones::BitSequenceReader& timestamp_reader,
                                            const BareBones::BitSequenceReader& values_reader,
                                            bool is_last_stalenan)
      : SeparatedTimestampValueDecodeIteratorTrait(samples_count, timestamp_reader, 0.0, is_last_stalenan), reader_(values_reader) {
    if (remaining_samples_ > 0) [[likely]] {
      decode_value();
      update_sample_value();
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

 private:
  friend Base;

  enum class DecoderType : uint8_t {
    kAscInteger,
    kValuesGorilla,
  };

  struct AscIntegerState {
    AscIntegerDecodeIterator::Decoder decoder;
    BareBones::Encoding::Gorilla::GorillaState gorilla_state{BareBones::Encoding::Gorilla::GorillaState::kFirstPoint};
  };

  struct ValuesGorillaState {
    ValuesGorillaDecodeIterator::Decoder decoder;
  };

  union {
    AscIntegerState asc_integer_{};
    ValuesGorillaState values_gorilla_;
  };
  BareBones::BitSequenceReader reader_;
  encoder::ValueType value_type_{encoder::ValueType::kValue};

  PROMPP_ALWAYS_INLINE bool decode() noexcept {
    if (decode_timestamp()) [[likely]] {
      decode_value();
      return true;
    }

    return false;
  }

  PROMPP_ALWAYS_INLINE void update_sample() noexcept {
    sample_.timestamp = decoded_timestamp();
    update_sample_value();
  }

  PROMPP_ALWAYS_INLINE void update_sample_value() noexcept {
    if (value_type_ == encoder::ValueType::kStaleNan) [[unlikely]] {
      sample_.value = BareBones::Encoding::Gorilla::STALE_NAN;
    } else if (value_type_ == encoder::ValueType::kSwitchToValuesGorillaMark) {
      sample_.value = values_gorilla_.decoder.value();
    } else {
      sample_.value = static_cast<double>(asc_integer_.decoder.timestamp());
    }
  }

  PROMPP_ALWAYS_INLINE void decode_value() noexcept {
    using enum encoder::ValueType;

    if (value_type_ != kSwitchToValuesGorillaMark) {
      if (value_type_ = asc_integer_.decoder.decode(reader_, asc_integer_.gorilla_state); value_type_ == kSwitchToValuesGorillaMark) [[unlikely]] {
        switch_to_values_gorilla();
      }
    } else {
      ValuesGorillaDecodeIterator::decode_value<ValuesGorillaDecodeIterator::SampleType::kOther>(values_gorilla_.decoder, reader_);
    }
  }

  PROMPP_ALWAYS_INLINE void switch_to_values_gorilla() noexcept {
    std::construct_at(&values_gorilla_);
    ValuesGorillaDecodeIterator::decode_value<ValuesGorillaDecodeIterator::SampleType::kFirst>(values_gorilla_.decoder, reader_);
  }
};

}  // namespace series_data::decoder
