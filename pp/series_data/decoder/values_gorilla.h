#pragma once

#include "series_data/encoder/value/values_gorilla.h"
#include "traits.h"

namespace series_data::decoder {

class ValuesGorillaDecodeIterator : public SeparatedTimestampValueDecodeIteratorTrait<ValuesGorillaDecodeIterator> {
 public:
  using Decoder = BareBones::Encoding::Gorilla::ValuesDecoder;

  enum class SampleType : uint8_t {
    kFirst = 0,
    kOther,
  };

  ValuesGorillaDecodeIterator(const encoder::BitSequenceWithItemsCount& timestamp_stream, const BareBones::BitSequenceReader& reader, bool is_last_stalenan)
      : ValuesGorillaDecodeIterator(timestamp_stream.count(), timestamp_stream.reader(), reader, is_last_stalenan) {}
  ValuesGorillaDecodeIterator(uint8_t samples_count,
                              const BareBones::BitSequenceReader& timestamp_reader,
                              const BareBones::BitSequenceReader& values_reader,
                              bool is_last_stalenan)
      : SeparatedTimestampValueDecodeIteratorTrait(samples_count, timestamp_reader, 0.0, is_last_stalenan), reader_(values_reader) {
    if (remaining_samples_ > 0) [[likely]] {
      decode_value<SampleType::kFirst>();
      sample_.value = decoder_.value();
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

 private:
  friend Base;

  BareBones::BitSequenceReader reader_;
  Decoder decoder_;

  PROMPP_ALWAYS_INLINE bool decode() noexcept {
    if (decode_timestamp()) [[likely]] {
      decode_value<SampleType::kOther>();
      return true;
    }

    return false;
  }

  PROMPP_ALWAYS_INLINE void update_sample() noexcept {
    sample_.timestamp = decoded_timestamp();
    sample_.value = decoder_.value();
  }

  template <SampleType Type>
  PROMPP_ALWAYS_INLINE void decode_value() noexcept {
    decode_value<Type>(decoder_, reader_);
  }
};

}  // namespace series_data::decoder
