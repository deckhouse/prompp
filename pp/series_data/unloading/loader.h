#pragma once

#include "bare_bones/bitset.h"
#include "bare_bones/encoding.h"
#include "bare_bones/stream_v_byte.h"
#include "series_data/data_storage.h"

namespace series_data::unloading {

struct SeriesToLoadInfo {
  series_data::encoder::CompactBitSequence buffer{};
  uint32_t chunk_id = 0;
};

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
        series_to_load_tmp_bitseqs_[ls_id];
        storage_.unused_series_bitmap.remove(ls_id);
      }
    }
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
    const uint32_t chunk_length_count = *reinterpret_cast<const uint32_t*>(buffer.data() + read_size_in_bytes);
    read_size_in_bytes += sizeof(uint32_t);
    std::cout << "chunk_length_sequence_size_in_bytes: " << chunk_length_sequence_size_in_bytes << '\n';
    std::cout << "chunk_length_count: " << chunk_length_count << '\n';
    const uint8_t* chunk_length_compact_sequence_ptr = reinterpret_cast<const uint8_t*>(buffer.data() + read_size_in_bytes);
    const EncodingChunkLengthSequence::sequence_type::DecodeIterator length_it_inner(
        chunk_length_compact_sequence_ptr, chunk_length_compact_sequence_ptr + EncodingChunkLengthSequence::sequence_type::kMaxKeySize, chunk_length_count);
    EncodingChunkLengthSequence::encoder_type encoder{};
    const EncodingChunkLengthSequence::Iterator length_it(length_it_inner, BareBones::StreamVByte::DecodeIteratorSentinel{}, &encoder);
    for (auto it = length_it; it != EncodingChunkLengthSequence::IteratorSentinel{}; ++it) {
      std::cout << (*it) << ' ';
    }
    std::cout << '\n';

    read_size_in_bytes += chunk_length_sequence_size_in_bytes;

    const uint32_t chunk_id_sequence_size_in_bytes = *reinterpret_cast<const uint32_t*>(buffer.data() + read_size_in_bytes);
    read_size_in_bytes += sizeof(uint32_t);
    const uint32_t chunk_id_sequence_count = *reinterpret_cast<const uint32_t*>(buffer.data() + read_size_in_bytes);
    read_size_in_bytes += sizeof(uint32_t);
    std::cout << "chunk_id_sequence_size_in_bytes: " << chunk_id_sequence_size_in_bytes << '\n';
    std::cout << "chunk_id_sequence_count: " << chunk_id_sequence_count << '\n';
    const uint8_t* chunk_id_compact_sequence_ptr = reinterpret_cast<const uint8_t*>(buffer.data() + read_size_in_bytes);
    const EncodingChunkIDSequence::sequence_type::DecodeIterator id_it_inner(
        chunk_id_compact_sequence_ptr, chunk_id_compact_sequence_ptr + EncodingChunkIDSequence::sequence_type::kMaxKeySize, chunk_id_sequence_count);
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
    const uint8_t* bitseqs_ptr = buffer.data() + read_size_in_bytes;
    BareBones::BitSequenceReader bitseqs_reader_tmp(bitseqs_ptr, BareBones::Bit::to_bits(bitseqs_size_in_bytes));
    std::cout << bitseqs_reader_tmp.position() << ' ' << bitseqs_reader_tmp.left() << '\n';

    std::cout << "total bytes read: " << read_size_in_bytes + bitseqs_size_in_bytes << '\n';

    for (auto& [ls_id, info] : series_to_load_tmp_bitseqs_) {
      auto& [bitseq, chunk_id] = info;

      auto find_it = std::ranges::find(bitset_it, BareBones::Bitset::IteratorSentinel{}, ls_id);
      if (find_it == BareBones::Bitset::IteratorSentinel{}) {
        std::cout << "ls_id: " << ls_id << " not found\n";
        continue;
      }
      uint32_t ls_id_bit_index = std::ranges::distance(bitset_it, find_it);

      uint32_t snapshot_ls_id_chunk_id = *std::ranges::next(id_it, ls_id_bit_index, EncodingChunkIDSequence::IteratorSentinel{});

      uint32_t bitseq_size_offset = 0;
      {
        auto it = length_it;
        for (uint32_t i = 0; i < ls_id_bit_index; ++i) {
          bitseq_size_offset += *it;
          ++it;
        }
      }
      uint32_t bitseq_size_in_bytes = *std::ranges::next(length_it, ls_id_bit_index, EncodingChunkLengthSequence::IteratorSentinel{});
      std::cout << "ls_id_bit_index: " << ls_id_bit_index << ", snapshot_ls_id_chunk_id: " << snapshot_ls_id_chunk_id << '\n';
      std::cout << "bitseq_size_offset: " << bitseq_size_offset << " ; bitseq_size_in_bytes: " << bitseq_size_in_bytes << '\n';

      if (snapshot_ls_id_chunk_id != chunk_id) {
        if (bitseq.size_in_bits() != 0) {
          std::cout << "bitseq.size_in_bits(): " << bitseq.size_in_bits() << '\n';
          load_chunk_id(ls_id, info);
        }
        chunk_id = snapshot_ls_id_chunk_id;
        bitseq.rewind();
      }

      BareBones::BitSequenceReader bitseqs_reader(bitseqs_ptr, BareBones::Bit::to_bits(bitseqs_size_in_bytes));
      for (uint32_t i = 0; i < bitseq_size_offset; ++i) {
        bitseqs_reader.ff(8);
      }
      for (uint32_t i = 0; i < bitseq_size_in_bytes; ++i) {
        std::cout << "bitseqs_reader.position(): " << bitseqs_reader.position() << " ; bitseqs_reader.left(): " << bitseqs_reader.left() << '\n';
        const uint32_t byte = bitseqs_reader.consume_bits_u32(8);
        bitseq.push_back_bits_u32(8, byte);
      }
    }

    for (auto& [ls_id, info] : series_to_load_tmp_bitseqs_) {
      auto& [bitseq, chunk_id] = info;
      std::cout << "ls_id: " << ls_id << " chunk_id: " << chunk_id << ' ' << bitseq.size_in_bits() << '\n';
    }
  }
  void load_finalize() {
    for (auto& [ls_id, info] : series_to_load_tmp_bitseqs_) {
      if (info.buffer.size_in_bits() != 0) {
        load_chunk_id(ls_id, info);
      }
    }

    Encoder<> encoder{storage_};
    OutdatedChunkMerger<decltype(encoder)> outdated_chunk_merger{encoder};
    outdated_chunk_merger.merge();
  }

 private:
  void load_chunk_id(uint32_t ls_id, SeriesToLoadInfo& info) const {
    const auto chunk_data =
        *std::ranges::next(DataStorage::SeriesChunkIterator{&storage_, ls_id}, info.chunk_id, series_data::DataStorage::SeriesChunks::end());

    auto& chunk_bit_sequence = [&]() -> encoder::CompactBitSequence& {
      if (chunk_data.is_open()) {
        return get_open_chunk_stream(ls_id);
      }
      return storage_.finalized_data_streams[chunk_data.chunk().encoder.external_index];
    }();

    std::cout << "bits in storage stream : " << chunk_bit_sequence.size_in_bits() << '\n';

    auto chunk_bit_sequence_reader = chunk_bit_sequence.reader();
    uint32_t chunk_bit_sequence_size_in_bits = chunk_bit_sequence.size_in_bits();

    for (uint32_t i = 0; i < BareBones::Bit::to_bytes(chunk_bit_sequence_size_in_bits); ++i) {
      const uint32_t byte = chunk_bit_sequence_reader.consume_bits_u32(8);
      info.buffer.push_back_bits_u32(8, byte);
    }
    const uint32_t last_bits = chunk_bit_sequence_reader.consume_bits_u32(chunk_bit_sequence_size_in_bits % 8);
    info.buffer.push_back_bits_u32(chunk_bit_sequence_size_in_bits % 8, last_bits);

    std::swap(info.buffer, chunk_bit_sequence);
  }

  [[nodiscard]] encoder::CompactBitSequence& get_open_chunk_stream(uint32_t ls_id) const noexcept {
    using enum EncodingType;

    const auto& chunk = storage_.open_chunks[ls_id];
    const auto encoding_type = storage_.open_chunks[ls_id].encoding_state.encoding_type;

    if (encoding_type == kAscInteger) {
      return storage_.get_asc_integer_stream<chunk::DataChunk::Type::kOpen>(chunk.encoder.external_index);
    }
    if (encoding_type == kValuesGorilla) {
      return storage_.get_values_gorilla_stream<chunk::DataChunk::Type::kOpen>(chunk.encoder.external_index);
    }
    // encoding_type == kAscIntegerThenValuesGorilla
    return storage_.get_asc_integer_then_values_gorilla_stream<chunk::DataChunk::Type::kOpen>(chunk.encoder.external_index);
  }

  DataStorage& storage_;
  std::map<uint32_t, SeriesToLoadInfo> series_to_load_tmp_bitseqs_;
};
}  // namespace series_data::unloading