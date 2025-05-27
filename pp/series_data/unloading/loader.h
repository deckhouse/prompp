#pragma once

#include "roaring/roaring.hh"

#include "bare_bones/bitset.h"
#include "bare_bones/encoding.h"
#include "bare_bones/stream_v_byte.h"
#include "series_data/data_storage.h"

namespace series_data::unloading {
class Loader {
  using EncodingChunkLengthSequence =
      BareBones::EncodedSequence<BareBones::Encoding::DeltaDeltaZigZag<BareBones::StreamVByte::CompactSequence<BareBones::StreamVByte::Codec0124Frequent0>>>;
  using EncodingChunkIDSequence =
      BareBones::EncodedSequence<BareBones::Encoding::RLE<BareBones::StreamVByte::CompactSequence<BareBones::StreamVByte::Codec0124Frequent0>>>;

 public:
  template <typename LsIDStorage>
  explicit Loader(DataStorage& storage, const LsIDStorage& ls_id_query) : storage_(storage) {
    for (const auto& ls_id : ls_id_query) {
      if (storage_.unused_series_bitmap.contains(ls_id)) {
        series_to_load_.add(ls_id);
        storage_.unused_series_bitmap.remove(ls_id);
      }
    }
    series_to_load_.runOptimize();
    series_to_load_.shrinkToFit();
  }

  void load_next(std::span<const uint8_t> buffer) {
    uint32_t read_size_in_bytes = 0;

    const uint32_t ls_id_bitmap_size_in_bits = *reinterpret_cast<const uint32_t*>(buffer.data());
    const uint32_t ls_id_bitmap_size_in_bytes = ((ls_id_bitmap_size_in_bits + 63) >> 6) * 8;
    std::cout << "ls_id_bitmap_size_in_bits: " << ls_id_bitmap_size_in_bits << '\n';
    std::cout << "ls_id_bitmap_size_in_bytes: " << ls_id_bitmap_size_in_bytes << '\n';
    read_size_in_bytes += sizeof(uint32_t);

    const uint64_t* bitset_ptr = reinterpret_cast<const uint64_t*>(buffer.data() + read_size_in_bytes);
    const BareBones::Bitset::Iterator bitset_it(bitset_ptr, ls_id_bitmap_size_in_bits);
    const uint32_t series_count = std::ranges::distance(bitset_it, BareBones::Bitset::IteratorSentinel{});
    std::cout << "series_count: " << series_count << '\n';
    for (auto it = bitset_it; it != BareBones::Bitset::IteratorSentinel{}; ++it) {
      std::cout << (*it) << ' ';
    }
    std::cout << '\n';

    read_size_in_bytes += ls_id_bitmap_size_in_bytes;

    const uint32_t chunk_length_sequence_size_in_bytes = *reinterpret_cast<const uint32_t*>(buffer.data() + read_size_in_bytes);
    read_size_in_bytes += sizeof(uint32_t);
    std::cout << "chunk_length_sequence_size_in_bytes: " << chunk_length_sequence_size_in_bytes << '\n';
    const uint8_t* chunk_length_compact_sequence_ptr = reinterpret_cast<const uint8_t*>(buffer.data() + read_size_in_bytes);
    const EncodingChunkLengthSequence::sequence_type::DecodeIterator length_it_inner(
        chunk_length_compact_sequence_ptr, chunk_length_compact_sequence_ptr + EncodingChunkLengthSequence::sequence_type::kMaxKeySize, series_count);
    EncodingChunkLengthSequence::encoder_type encoder{};
    const EncodingChunkLengthSequence::Iterator length_it(length_it_inner, BareBones::StreamVByte::DecodeIteratorSentinel{}, &encoder);
    for (auto it = length_it; it != EncodingChunkLengthSequence::IteratorSentinel{}; ++it) {
      std::cout << (*it) << ' ';
    }
    std::cout << '\n';

    read_size_in_bytes += chunk_length_sequence_size_in_bytes;

    const uint32_t chunk_id_sequence_size_in_bytes = *reinterpret_cast<const uint32_t*>(buffer.data() + read_size_in_bytes);
    read_size_in_bytes += sizeof(uint32_t);
    std::cout << "chunk_id_sequence_size_in_bytes: " << chunk_id_sequence_size_in_bytes << '\n';
    const uint8_t* chunk_id_compact_sequence_ptr = reinterpret_cast<const uint8_t*>(buffer.data() + read_size_in_bytes);
    const EncodingChunkIDSequence::sequence_type::DecodeIterator id_it_inner(
        chunk_id_compact_sequence_ptr, chunk_id_compact_sequence_ptr + EncodingChunkIDSequence::sequence_type::kMaxKeySize, series_count);
    EncodingChunkIDSequence::encoder_type id_encoder{};
    const EncodingChunkIDSequence::Iterator id_it(id_it_inner, BareBones::StreamVByte::DecodeIteratorSentinel{}, &id_encoder);
    for (auto it = id_it; it != EncodingChunkIDSequence::IteratorSentinel{}; ++it) {
      std::cout << (*it) << ' ';
    }
    std::cout << '\n';

    read_size_in_bytes += chunk_id_sequence_size_in_bytes;

    const uint32_t bitseqs_size_in_bytes = *reinterpret_cast<const uint32_t*>(buffer.data() + read_size_in_bytes);
    read_size_in_bytes += sizeof(uint32_t);
    std::cout << "bitseqs_size_in_bytes: " << bitseqs_size_in_bytes << '\n';
    BareBones::BitSequenceReader bitseqs_reader(buffer.data() + read_size_in_bytes, BareBones::Bit::to_bits(bitseqs_size_in_bytes));
    std::cout << bitseqs_reader.position() << ' ' << bitseqs_reader.left() << '\n';
  }
  void load_finalize();

 private:
  DataStorage& storage_;
  roaring::Roaring series_to_load_{};
};
}  // namespace series_data::unloading