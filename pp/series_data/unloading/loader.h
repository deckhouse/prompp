#pragma once

#include "common.h"

#include "bare_bones/bitset.h"
#include "bare_bones/encoding.h"
#include "bare_bones/preprocess.h"
#include "series_data/concepts.h"
#include "series_data/data_storage.h"
#include "series_data/encoder.h"

namespace series_data::unloading {

struct SeriesToLoadInfo {
  encoder::CompactBitSequence buffer{};
  uint32_t chunk_id = 0;
};

class Loader {
 public:
  explicit Loader(DataStorage& storage) : storage_(storage) {
    series_to_load_tmp_bitseqs_.reserve(storage_.unloaded_series_bitmap.popcount());
    for (const auto& ls_id : storage_.unloaded_series_bitmap) {
      series_to_load_tmp_bitseqs_.try_emplace(ls_id);
    }
    storage_.unloaded_series_bitmap.clear();
  }

  template <LsIDStorageInterface LsIDStorage>
  explicit Loader(DataStorage& storage, const LsIDStorage& ls_id_range, uint32_t ls_id_range_count) : storage_(storage) {
    series_to_load_tmp_bitseqs_.reserve(ls_id_range_count);
    for (const auto& ls_id : ls_id_range) {
      if (storage_.unloaded_series_bitmap.is_set(ls_id)) {
        storage_.unloaded_series_bitmap.reset(ls_id);
        series_to_load_tmp_bitseqs_.try_emplace(ls_id);
      }
    }
  }

  void load_next(std::span<const uint8_t> buffer) {
    const auto bitset_it = parse_ls_id_bitmap(buffer);
    const auto length_it = parse_encoded_sequence<EncodingChunkLengthSequence>(buffer);
    const auto id_it = parse_encoded_sequence<EncodingChunkIDSequence>(buffer);

    const uint8_t* bitseqs_ptr = buffer.data();

    process_ls_id_data(bitset_it, length_it, id_it, bitseqs_ptr);
  }

  template <EncoderInterface Encoder = series_data::Encoder<>>
  void load_finalize() {
    for (auto& [ls_id, info] : series_to_load_tmp_bitseqs_) {
      if (info.buffer.size_in_bits() != 0) {
        load_chunk_id(ls_id, info);
      }
    }

    Encoder encoder{storage_};
    OutdatedChunkMerger<Encoder> outdated_chunk_merger{encoder};
    for (const auto& ls_id : std::views::keys(series_to_load_tmp_bitseqs_)) {
      outdated_chunk_merger.merge(ls_id);
      storage_.outdated_chunks.erase(ls_id);
    }
  }

  [[nodiscard]] bool empty() const noexcept { return series_to_load_tmp_bitseqs_.empty(); }

 private:
  PROMPP_ALWAYS_INLINE static uint32_t read_u32(std::span<const uint8_t>& buffer) noexcept {
    assert(buffer.size() >= sizeof(uint32_t));

    uint32_t val = 0;
    std::memcpy(&val, buffer.data(), sizeof(uint32_t));
    buffer = buffer.subspan(sizeof(uint32_t));

    return val;
  }

  PROMPP_ALWAYS_INLINE static void read_data(const uint8_t* bitseq_ptr, uint32_t byte_count, encoder::CompactBitSequence& output) noexcept {
    const uint32_t output_size_in_bytes = output.size_in_bytes();
    output.push_back_single_zero_bit(BareBones::Bit::to_bits(byte_count));
    std::memcpy(output.raw_bytes() + output_size_in_bytes, bitseq_ptr, byte_count);
  }

  static BareBones::Bitset::Iterator parse_ls_id_bitmap(std::span<const uint8_t>& buffer) noexcept {
    const uint32_t bit_count = read_u32(buffer);
    const uint32_t byte_count = BareBones::Bit::to_ceil_units<uint64_t>(bit_count) * sizeof(uint64_t);

    const std::span bit_data(reinterpret_cast<const uint64_t*>(buffer.data()), BareBones::Bit::to_ceil_units<uint64_t>(bit_count));
    buffer = buffer.subspan(byte_count);

    return BareBones::Bitset::read_iterator(bit_data, bit_count);
  }

  template <typename EncodedSequence>
  typename EncodedSequence::const_iterator_type parse_encoded_sequence(std::span<const uint8_t>& buffer) const noexcept {
    const uint32_t byte_count = read_u32(buffer);
    const uint32_t elem_count = read_u32(buffer);

    const std::span compact_data(buffer.data(), byte_count);
    buffer = buffer.subspan(byte_count);

    if constexpr (std::is_same_v<EncodedSequence, EncodingChunkLengthSequence>) {
      return EncodedSequence::read_iterator(compact_data, elem_count, length_encoder_);
    } else {
      return EncodedSequence::read_iterator(compact_data, elem_count, id_encoder_);
    }
  }

  void process_ls_id_data(BareBones::Bitset::Iterator bitset_it,
                          EncodingChunkLengthSequence::Iterator length_it,
                          EncodingChunkIDSequence::Iterator id_it,
                          const uint8_t* bitseqs_ptr) {
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
        info.buffer.push_back_bytes(bitseqs_ptr + accumulated_offset, bitseq_size);
      }

      accumulated_offset += *length_it;

      ++bitset_it;
      ++length_it;
      ++id_it;
    }
  }

  void load_chunk_id(uint32_t ls_id, SeriesToLoadInfo& info) const {
    const auto& chunk_data =
        std::ranges::next(DataStorage::SeriesChunkIterator{&storage_, ls_id}, info.chunk_id, series_data::DataStorage::SeriesChunks::end());

    auto& chunk_bit_sequence = [&]() -> encoder::CompactBitSequence& {
      if (chunk_data->is_open()) {
        return get_open_chunk_stream(storage_, ls_id);
      }
      return storage_.finalized_data_streams[chunk_data->chunk().encoder.external_index];
    }();

    info.buffer.push_back_bytes(chunk_bit_sequence.raw_bytes(), chunk_bit_sequence.size_in_bytes());

    chunk_bit_sequence = std::move(info.buffer);
  }

  DataStorage& storage_;

  EncodingChunkLengthSequence::encoder_type length_encoder_{};
  EncodingChunkIDSequence::encoder_type id_encoder_{};

  phmap::flat_hash_map<uint32_t, SeriesToLoadInfo> series_to_load_tmp_bitseqs_{};
};
}  // namespace series_data::unloading