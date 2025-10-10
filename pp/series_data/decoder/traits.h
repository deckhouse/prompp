#pragma once

#include "bare_bones/preprocess.h"
#include "series_data/encoder/bit_sequence.h"
#include "series_data/encoder/sample.h"
#include "series_data/encoder/timestamp/encoder.h"

namespace series_data::decoder {

class DecodeIteratorSentinel {};

class DecodeIteratorTypeTrait {
 public:
  using iterator_category = std::forward_iterator_tag;
  using value_type = encoder::Sample;
  using difference_type = ptrdiff_t;
  using pointer = encoder::Sample*;
  using reference = encoder::Sample&;
};

template <std::unsigned_integral SampleCountType>
class DecodeIteratorTrait : public DecodeIteratorTypeTrait {
 public:
  explicit DecodeIteratorTrait(SampleCountType count) : remaining_samples_{count} {}
  explicit DecodeIteratorTrait(double value, SampleCountType count) : sample_{.value = value}, remaining_samples_{count} {}
  explicit DecodeIteratorTrait(double value, SampleCountType count, bool last_stalenan)
      : sample_{.value = value}, remaining_samples_{count}, last_stalenan_{last_stalenan} {}

  const encoder::Sample& operator*() const noexcept { return sample_; }
  const encoder::Sample* operator->() const noexcept { return &sample_; }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const noexcept { return remaining_samples_ == 0; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE SampleCountType remaining_samples() const noexcept { return remaining_samples_; }

 protected:
  encoder::Sample sample_;
  SampleCountType remaining_samples_{};
  bool last_stalenan_{false};
};

class SeparatedTimestampValueDecodeIteratorTrait : public DecodeIteratorTrait<uint8_t> {
 public:
  SeparatedTimestampValueDecodeIteratorTrait(uint8_t samples_count, const BareBones::BitSequenceReader& timestamp_reader, double value, bool last_stalenan)
      : DecodeIteratorTrait(value, samples_count, last_stalenan), timestamp_decoder_(timestamp_reader) {
    if (remaining_samples_ > 0) {
      sample_.timestamp = timestamp_decoder_.decode();
    }
  }
  explicit SeparatedTimestampValueDecodeIteratorTrait(const encoder::BitSequenceWithItemsCount& timestamp_stream)
      : SeparatedTimestampValueDecodeIteratorTrait(timestamp_stream.count(), timestamp_stream.reader(), 0.0, false) {}
  SeparatedTimestampValueDecodeIteratorTrait(const encoder::BitSequenceWithItemsCount& timestamp_stream, double value)
      : SeparatedTimestampValueDecodeIteratorTrait(timestamp_stream.count(), timestamp_stream.reader(), value, false) {}
  SeparatedTimestampValueDecodeIteratorTrait(const encoder::BitSequenceWithItemsCount& timestamp_stream, double value, bool last_stalenan)
      : SeparatedTimestampValueDecodeIteratorTrait(timestamp_stream.count(), timestamp_stream.reader(), value, last_stalenan) {}

  PROMPP_ALWAYS_INLINE bool decode_timestamp() noexcept {
    if (--remaining_samples_ > 0) {
      sample_.timestamp = timestamp_decoder_.decode();
      return true;
    }

    return false;
  }

 protected:
  encoder::timestamp::TimestampDecoder timestamp_decoder_;
};

}  // namespace series_data::decoder