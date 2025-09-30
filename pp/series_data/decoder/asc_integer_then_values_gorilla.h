#pragma once

#include "asc_integer.h"
#include "values_gorilla.h"

namespace series_data::decoder {

class AscIntegerThenValuesGorillaDecodeIterator : public SeparatedTimestampValueDecodeIteratorTrait {
 public:
  AscIntegerThenValuesGorillaDecodeIterator(const encoder::BitSequenceWithItemsCount& timestamp_stream,
                                            const BareBones::BitSequenceReader& reader,
                                            bool is_last_stalenan)
      : AscIntegerThenValuesGorillaDecodeIterator(timestamp_stream.count(), timestamp_stream.reader(), reader, is_last_stalenan) {}
  AscIntegerThenValuesGorillaDecodeIterator(uint8_t samples_count,
                                            const BareBones::BitSequenceReader& timestamp_reader,
                                            const BareBones::BitSequenceReader& values_reader,
                                            bool is_last_stalenan)
      : SeparatedTimestampValueDecodeIteratorTrait(samples_count, timestamp_reader, 0.0, is_last_stalenan), reader_(values_reader) {
    if (remaining_samples_ > 0) {
      decode_value();
    }
  }

  PROMPP_ALWAYS_INLINE AscIntegerThenValuesGorillaDecodeIterator& operator++() noexcept {
    if (decode_timestamp()) {
      decode_value();
    }
    return *this;
  }

  PROMPP_ALWAYS_INLINE AscIntegerThenValuesGorillaDecodeIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

 private:
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
  DecoderType decoder_type_{DecoderType::kAscInteger};

  PROMPP_ALWAYS_INLINE void decode_value() noexcept {
    if (decoder_type_ == DecoderType::kAscInteger) {
      using enum encoder::ValueType;
      if (asc_integer_.decoder.decode(reader_, asc_integer_.gorilla_state, sample_.value) == kSwitchToValuesGorillaMark) [[unlikely]] {
        switch_to_values_gorilla();
      }
    } else {
      sample_.value = ValuesGorillaDecodeIterator::decode_value<false>(values_gorilla_.decoder, reader_);
    }
  }

  PROMPP_ALWAYS_INLINE void switch_to_values_gorilla() noexcept {
    std::construct_at(&values_gorilla_);
    sample_.value = ValuesGorillaDecodeIterator::decode_value<true>(values_gorilla_.decoder, reader_);
    decoder_type_ = DecoderType::kValuesGorilla;
  }
};

}  // namespace series_data::decoder
