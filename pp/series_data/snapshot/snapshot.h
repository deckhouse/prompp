#pragma once

#include <roaring/cpp/roaring.hh>

#include "bare_bones/encoding.h"
#include "series_data/encoder/bit_sequence.h"

namespace series_data::snapshot {
class Snapshot {
  using EncodingLengthSequence =
      BareBones::EncodedSequence<BareBones::Encoding::DeltaDeltaZigZag<BareBones::StreamVByte::Sequence<BareBones::StreamVByte::Codec0124Frequent0>>>;

 public:
  void encode(uint32_t ls_id, const encoder::CompactBitSequence& sequence) noexcept {
    ls_id_bitmap_.add(ls_id);
    length_sequence_.push_back(sequence.size_in_bits());
    sequence.write_to(bitseq_buffer_);
  }

 private:
  EncodingLengthSequence length_sequence_{};
  roaring::Roaring ls_id_bitmap_{};
  BareBones::ShrinkedToFitOStringStream bitseq_buffer_{};
};
}  // namespace series_data::snapshot