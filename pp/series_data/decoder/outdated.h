#pragma once

#include "traits.h"

namespace series_data::decoder {

class OutdatedDecodeIterator : public DecodeIteratorTypeTrait {
 public:
  explicit OutdatedDecodeIterator(const chunk::OutdatedChunk& chunk) : reader_(chunk.stream().reader()), remaining_samples_{chunk.samples_count()} { decode(); }

  PROMPP_ALWAYS_INLINE OutdatedDecodeIterator& operator++() noexcept {
    --remaining_samples_;
    decode();
    return *this;
  }

  PROMPP_ALWAYS_INLINE OutdatedDecodeIterator operator++(int) noexcept {
    const auto result = *this;
    ++*this;
    return result;
  }

  const encoder::Sample& operator*() const noexcept { return sample_; }
  const encoder::Sample* operator->() const noexcept { return &sample_; }

  PROMPP_ALWAYS_INLINE bool operator==(const DecodeIteratorSentinel&) const noexcept { return remaining_samples_ == 0; }

 private:
  using Decoder = BareBones::Encoding::Gorilla::ValuesDecoder;

  encoder::Sample sample_{};

  BareBones::BitSequenceReader reader_;
  BareBones::Encoding::Gorilla::StreamDecoder<BareBones::Encoding::Gorilla::ZigZagTimestampDecoder<>, BareBones::Encoding::Gorilla::ValuesDecoder> decoder_;

  uint32_t remaining_samples_;

  PROMPP_ALWAYS_INLINE void decode() noexcept {
    if (remaining_samples_ > 0) [[likely]] {
      decoder_.decode(reader_, reader_);
      sample_.value = decoder_.last_value();
      sample_.timestamp = decoder_.last_timestamp();
    }
  }
};

}  // namespace series_data::decoder
