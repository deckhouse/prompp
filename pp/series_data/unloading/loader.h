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
    const auto bitset_it = parse_ls_id_bitmap(buffer);
    const auto length_it = parse_encoded_sequence<EncodingChunkLengthSequence>(buffer);
    const auto id_it = parse_encoded_sequence<EncodingChunkIDSequence>(buffer);

    const uint32_t bitseqs_total_size = read_u32(buffer);
    const uint8_t* bitseqs_ptr = buffer.data();

    process_ls_id_data(bitset_it, length_it, id_it, bitseqs_ptr, bitseqs_total_size);
  }

  template <EncoderInterface Encoder>
  void load_finalize() {
    for (auto& [ls_id, info] : series_to_load_tmp_bitseqs_) {
      if (info.buffer.size_in_bits() != 0) {
        load_chunk_id(ls_id, info);
      }
    }

    Encoder encoder{storage_};
    OutdatedChunkMerger<Encoder> outdated_chunk_merger{encoder};
    outdated_chunk_merger.merge();
  }

 private:
  PROMPP_ALWAYS_INLINE static uint32_t read_u32(std::span<const uint8_t>& buffer) noexcept {
    assert(buffer.size() >= sizeof(uint32_t));

    uint32_t val = 0;
    std::memcpy(&val, buffer.data(), sizeof(uint32_t));
    buffer = buffer.subspan(sizeof(uint32_t));

    return val;
  }

  PROMPP_ALWAYS_INLINE static void skip_bytes(BareBones::BitSequenceReader& reader, uint32_t count) noexcept { reader.ff(BareBones::Bit::to_bits(count)); }

  PROMPP_ALWAYS_INLINE static void read_data(const uint8_t* bitseq_ptr, uint32_t byte_count, encoder::CompactBitSequence& output) noexcept {
    const uint32_t output_size_in_bytes = output.size_in_bytes();
    output.push_back_single_zero_bit(BareBones::Bit::to_bits(byte_count));
    std::memcpy(output.raw_bytes() + output_size_in_bytes, bitseq_ptr, byte_count);
  }

  static BareBones::Bitset::Iterator parse_ls_id_bitmap(std::span<const uint8_t>& buffer) noexcept {
    const uint32_t bit_count = read_u32(buffer);
    const uint32_t byte_count = BareBones::Bit::to_ceil_units<uint64_t>(bit_count) * sizeof(uint64_t);

    const auto* bit_data = reinterpret_cast<const uint64_t*>(buffer.data());
    buffer = buffer.subspan(byte_count);

    return BareBones::Bitset::Iterator{bit_data, bit_count};
  }

  template <typename EncodedSequence>
  typename EncodedSequence::const_iterator_type parse_encoded_sequence(std::span<const uint8_t>& buffer) const noexcept {
    const uint32_t byte_count = read_u32(buffer);
    const uint32_t elem_count = read_u32(buffer);

    const auto* compact_data = buffer.data();
    buffer = buffer.subspan(byte_count);

    auto inner = typename EncodedSequence::sequence_type::DecodeIterator(compact_data, compact_data + EncodedSequence::sequence_type::kMaxKeySize, elem_count);

    if constexpr (std::is_same_v<EncodedSequence, EncodingChunkLengthSequence>) {
      return typename EncodedSequence::const_iterator_type(inner, {}, &length_encoder_);
    } else {
      return typename EncodedSequence::const_iterator_type(inner, {}, &id_encoder_);
    }
  }

  void process_ls_id_data(BareBones::Bitset::Iterator bitset_it,
                          EncodingChunkLengthSequence::Iterator length_it,
                          EncodingChunkIDSequence::Iterator id_it,
                          const uint8_t* bitseqs_ptr,
                          uint32_t total_size) {
    BareBones::BitSequenceReader reader(bitseqs_ptr, BareBones::Bit::to_bits(total_size));
    uint32_t accumulated_offset = 0;

    while (bitset_it != BareBones::Bitset::IteratorSentinel{}) {
      const uint32_t ls_id = *bitset_it;

      auto series_it = series_to_load_tmp_bitseqs_.find(ls_id);

      if (series_it != series_to_load_tmp_bitseqs_.end()) {
        auto& info = series_it->second;

        const uint32_t chunk_id_snapshot = *id_it;
        const uint32_t bitseq_size = *length_it;

        if (chunk_id_snapshot != info.chunk_id) {
          if (info.buffer.size_in_bits() != 0) {
            load_chunk_id(ls_id, info);
          }
          info.chunk_id = chunk_id_snapshot;
          info.buffer.rewind();
        }
        read_data(bitseqs_ptr + accumulated_offset, bitseq_size, info.buffer);
      }

      accumulated_offset += *length_it;

      ++bitset_it;
      ++length_it;
      ++id_it;
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

    const uint32_t info_buffer_size_in_bytes = info.buffer.size_in_bytes();
    info.buffer.push_back_single_zero_bit(chunk_bit_sequence.size_in_bits());
    memcpy(info.buffer.raw_bytes() + info_buffer_size_in_bytes, chunk_bit_sequence.raw_bytes(), chunk_bit_sequence.size_in_bytes());

    chunk_bit_sequence = std::move(info.buffer);
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

  EncodingChunkLengthSequence::encoder_type length_encoder_{};
  EncodingChunkIDSequence::encoder_type id_encoder_{};

  std::map<uint32_t, SeriesToLoadInfo> series_to_load_tmp_bitseqs_;
};
}  // namespace series_data::unloading