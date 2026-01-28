#pragma once

#include "series_data/encoder/value/asc_integer.h"
#include "traits.h"

namespace series_data::decoder {

class AscIntegerDecodeIterator : public SeparatedTimestampValueDecodeIteratorTrait<AscIntegerDecodeIterator> {
 public:
  using Decoder = encoder::ZigZagTimestampDecoder;

  template <class BitSequenceWithItemsCount>
  AscIntegerDecodeIterator(const BitSequenceWithItemsCount& timestamp_stream, const BareBones::BitSequenceReader& reader, bool is_last_stalenan)
      : AscIntegerDecodeIterator(timestamp_stream.count(), timestamp_stream.reader(), reader, is_last_stalenan) {}
  AscIntegerDecodeIterator(uint8_t samples_count,
                           const BareBones::BitSequenceReader& timestamp_reader,
                           const BareBones::BitSequenceReader& values_reader,
                           bool is_last_stalenan)
      : SeparatedTimestampValueDecodeIteratorTrait(samples_count, timestamp_reader, 0.0, is_last_stalenan), reader_(values_reader) {
    if (remaining_samples_ > 0) [[likely]] {
      decode_value();
      sample_.value = decoded_value();
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
    if (value_type_ == encoder::ValueType::kStaleNan) [[unlikely]] {
      return BareBones::Encoding::Gorilla::STALE_NAN;
    }

    return static_cast<double>(decoder_.timestamp());
  }

 private:
  friend Base;

  using GorillaState = BareBones::Encoding::Gorilla::GorillaState;

  Decoder decoder_;
  BareBones::BitSequenceReader reader_;
  GorillaState gorilla_state_{GorillaState::kFirstPoint};
  encoder::ValueType value_type_{encoder::ValueType::kValue};

  PROMPP_ALWAYS_INLINE bool decode() noexcept {
    if (decode_timestamp()) [[likely]] {
      decode_value();
      return true;
    }

    return false;
  }

  PROMPP_ALWAYS_INLINE void decode_value() noexcept { value_type_ = decoder_.decode(reader_, gorilla_state_); }

  PROMPP_ALWAYS_INLINE void update_sample() noexcept {
    sample_.timestamp = decoded_timestamp();
    sample_.value = decoded_value();
  }
};

}  // namespace series_data::decoder
