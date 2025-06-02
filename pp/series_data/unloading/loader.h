#pragma once

#include "bare_bones/bitset.h"
#include "bare_bones/encoding.h"
#include "bare_bones/preprocess.h"
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
    uint32_t offset = 0;

    const auto bitset_it = parse_ls_id_bitmap(buffer, offset);
    const auto length_it = parse_encoded_sequence<EncodingChunkLengthSequence>(buffer, offset);
    const auto id_it = parse_encoded_sequence<EncodingChunkIDSequence>(buffer, offset);

    const uint32_t bitseqs_total_size = read_u32(buffer, offset);
    const uint8_t* bitseqs_ptr = buffer.data() + offset;

    process_ls_id_data(bitset_it, length_it, id_it, bitseqs_ptr, bitseqs_total_size);
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
  PROMPP_ALWAYS_INLINE static uint32_t read_u32(std::span<const uint8_t> buffer, uint32_t& offset) noexcept {
    uint32_t val = 0;
    std::memcpy(&val, buffer.data() + offset, sizeof(uint32_t));
    offset += sizeof(uint32_t);
    return val;
  }

  PROMPP_ALWAYS_INLINE static auto find_ls_id(const BareBones::Bitset::Iterator& begin, uint32_t ls_id) noexcept {
    return std::ranges::find(begin, BareBones::Bitset::IteratorSentinel{}, ls_id);
  }

  template <typename EncodedSequence>
  PROMPP_ALWAYS_INLINE static uint32_t nth_value_in_encoded_sequence(typename EncodedSequence::const_iterator_type begin, uint32_t n) noexcept {
    return *std::ranges::next(begin, n, typename EncodedSequence::sentinel{});
  }

  template <typename It>
  PROMPP_ALWAYS_INLINE static uint32_t accumulate_sequence_prefix(It it, uint32_t upto) noexcept {
    uint32_t sum = 0;
    for (uint32_t i = 0; i < upto; ++i) {
      sum += *it;
      ++it;
    }
    return sum;
  }

  PROMPP_ALWAYS_INLINE static void skip_bytes(BareBones::BitSequenceReader& reader, uint32_t count) noexcept {
    for (uint32_t i = 0; i < count; ++i) {
      reader.ff(8);
    }
  }

  PROMPP_ALWAYS_INLINE static void read_data(BareBones::BitSequenceReader& reader, uint32_t count, encoder::CompactBitSequence& output) noexcept {
    for (uint32_t i = 0; i < count; ++i) {
      output.push_back_bits_u32(8, reader.consume_bits_u32(8));
    }
  }

  PROMPP_ALWAYS_INLINE static uint32_t get_bitset_iterator_index(const BareBones::Bitset::Iterator& begin, const BareBones::Bitset::Iterator& it) {
    return static_cast<uint32_t>(std::ranges::distance(begin, it));
  }

  static BareBones::Bitset::Iterator parse_ls_id_bitmap(std::span<const uint8_t> buffer, uint32_t& offset) noexcept {
    const uint32_t bit_count = read_u32(buffer, offset);
    const uint32_t byte_count = ((bit_count + 63) >> 6) * 8;

    const auto* bit_data = reinterpret_cast<const uint64_t*>(buffer.data() + offset);
    offset += byte_count;

    return BareBones::Bitset::Iterator{bit_data, bit_count};
  }

  template <typename EncodedSequence>
  static typename EncodedSequence::const_iterator_type parse_encoded_sequence(std::span<const uint8_t> buffer, uint32_t& offset) noexcept {
    const uint32_t byte_size = read_u32(buffer, offset);
    const uint32_t elem_count = read_u32(buffer, offset);

    const auto* compact_data = buffer.data() + offset;
    offset += byte_size;

    auto inner = typename EncodedSequence::sequence_type::DecodeIterator(compact_data, compact_data + EncodedSequence::sequence_type::kMaxKeySize, elem_count);

    static thread_local typename EncodedSequence::encoder_type encoder;
    return typename EncodedSequence::const_iterator_type(inner, {}, &encoder);
  }

  void process_ls_id_data(const BareBones::Bitset::Iterator& bitset_it,
                          EncodingChunkLengthSequence::Iterator length_it,
                          EncodingChunkIDSequence::Iterator id_it,
                          const uint8_t* bitseqs_ptr,
                          uint32_t total_size) {
    for (auto& [ls_id, info] : series_to_load_tmp_bitseqs_) {
      const auto it = find_ls_id(bitset_it, ls_id);
      if (it == BareBones::Bitset::IteratorSentinel{}) {
        continue;
      }

      const uint32_t index = get_bitset_iterator_index(bitset_it, it);
      const uint32_t chunk_id_snapshot = nth_value_in_encoded_sequence<EncodingChunkIDSequence>(id_it, index);
      const uint32_t bitseq_size = nth_value_in_encoded_sequence<EncodingChunkLengthSequence>(length_it, index);
      const uint32_t offset = accumulate_sequence_prefix(length_it, index);

      if (chunk_id_snapshot != info.chunk_id) {
        if (info.buffer.size_in_bits() != 0) {
          load_chunk_id(ls_id, info);
        }
        info.chunk_id = chunk_id_snapshot;
        info.buffer.rewind();
      }

      BareBones::BitSequenceReader reader(bitseqs_ptr, BareBones::Bit::to_bits(total_size));
      skip_bytes(reader, offset);
      read_data(reader, bitseq_size, info.buffer);
    }
  }

  void load_chunk_id(uint32_t ls_id, SeriesToLoadInfo& info) const {
    const auto chunk_data =
        *std::ranges::next(DataStorage::SeriesChunkIterator{&storage_, ls_id}, info.chunk_id, series_data::DataStorage::SeriesChunks::end());

    auto& chunk_bit_sequence = [&]() -> encoder::CompactBitSequence& {
      if (chunk_data.is_open()) {
        return get_open_chunk_stream(ls_id);
      }
      return storage_.finalized_data_streams[chunk_data.chunk().encoder.external_index];
    }();

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