#pragma once

#include "common.h"

#include "bare_bones/bitset.h"
#include "bare_bones/encoding.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/type_traits.h"
#include "series_data/concepts.h"
#include "series_data/data_storage.h"
#include "series_data/encoder.h"
#include "series_data/outdated_chunk_merger.h"

namespace series_data::unloading {
struct SeriesToLoadInfo {
  uint32_t ls_id = 0;
  uint32_t chunk_id = 0;
  encoder::CompactBitSequence buffer{};
};
}  // namespace series_data::unloading

template <>
struct BareBones::IsTriviallyReallocatable<series_data::unloading::SeriesToLoadInfo> : std::true_type {};

namespace series_data::unloading {
class Loader {
 public:
  explicit Loader(DataStorage& storage) : storage_(storage) {
    series_to_load_infos_.resize(storage_.unloaded_series_bitmap.popcount());
    std::ranges::for_each(std::views::zip(storage_.unloaded_series_bitmap, series_to_load_infos_), [](auto pair) {
      auto [ls_id, info] = pair;
      info.ls_id = ls_id;
    });
    storage_.unloaded_series_bitmap.clear();
  }

  template <LsIDStorageInterface LsIDStorage>
  explicit Loader(DataStorage& storage, const LsIDStorage& ls_id_range, uint32_t ls_id_range_count) : storage_(storage) {
    series_to_load_infos_.resize(ls_id_range_count);
    std::ranges::for_each(std::views::zip(ls_id_range, series_to_load_infos_), [&](auto pair) {
      auto [ls_id, info] = pair;
      info.ls_id = ls_id;

      storage_.unloaded_series_bitmap.reset(ls_id);
    });
  }

  void load_next(std::span<const uint8_t> buffer) {
    if (buffer.size() != *sizes_it_++) {
      throw BareBones::Exception(0x16d2a1e15cfa347d, "Loader::load_next: Buffer size mismatch");
    }

    const auto bitset_it = BareBones::Bitset::create_read_iterator(buffer);
    const auto length_it = EncodingChunkLengthSequence::create_read_iterator(buffer, length_encoder_);
    const auto id_it = EncodingChunkIDSequence::create_read_iterator(buffer, id_encoder_);

    const uint8_t* bitseqs_ptr = buffer.data();

    infos_it_ = series_to_load_infos_.begin();

    process_ls_id_data(bitset_it, length_it, id_it, bitseqs_ptr);
  }

  template <EncoderInterface Encoder = series_data::Encoder<>>
  void load_finalize() {
    for (auto& info : series_to_load_infos_) {
      if (info.buffer.size_in_bits() != 0) {
        load_chunk_id(info);
      }
    }

    Encoder encoder{storage_};
    OutdatedChunkMerger<Encoder> outdated_chunk_merger{encoder};
    for (const auto& info : series_to_load_infos_) {
      outdated_chunk_merger.merge(info.ls_id);
    }
  }

  [[nodiscard]] bool empty() const noexcept { return series_to_load_infos_.empty(); }

 private:
  void process_ls_id_data(BareBones::Bitset::Iterator bitset_it,
                          EncodingChunkLengthSequence::Iterator length_it,
                          EncodingChunkIDSequence::Iterator id_it,
                          const uint8_t* bitseqs_ptr) {
    uint32_t accumulated_offset = 0;

    const auto infos_end = series_to_load_infos_.end();
    while (bitset_it != BareBones::Bitset::IteratorSentinel{} && infos_it_ != infos_end) {
      const uint32_t ls_id = *bitset_it;

      find_ls_id_info(ls_id);

      if (infos_it_ != infos_end && infos_it_->ls_id == ls_id) {
        auto& info = *infos_it_;

        const uint32_t chunk_id_snapshot = *id_it;
        const uint32_t bitseq_size = *length_it;

        if (chunk_id_snapshot != info.chunk_id) {
          if (info.buffer.size_in_bits() != 0) {
            load_chunk_id(info);
          }
          info.chunk_id = chunk_id_snapshot;
          info.buffer.rewind();
        }
        info.buffer.push_back_bytes(bitseqs_ptr + accumulated_offset, BareBones::Bit::to_bits(bitseq_size));
      }

      accumulated_offset += *length_it;

      ++bitset_it;
      ++length_it;
      ++id_it;
    }
  }

  void PROMPP_ALWAYS_INLINE find_ls_id_info(uint32_t ls_id) noexcept {
    const auto infos_end = series_to_load_infos_.end();
    while (infos_it_ != infos_end && infos_it_->ls_id < ls_id) {
      ++infos_it_;
    }
  }

  void load_chunk_id(SeriesToLoadInfo& info) const {
    const auto& chunk_data = std::ranges::next(DataStorage::SeriesChunkIterator{&storage_, info.ls_id}, info.chunk_id, DataStorage::SeriesChunks::end());

    auto& chunk_bit_sequence = [&]() -> encoder::CompactBitSequence& {
      if (chunk_data->is_open()) {
        return get_open_chunk_stream(storage_, info.ls_id);
      }
      return storage_.finalized_data_streams[chunk_data->chunk().encoder.external_index];
    }();

    info.buffer.push_back_bytes(chunk_bit_sequence.raw_bytes(), chunk_bit_sequence.size_in_bits());

    chunk_bit_sequence = std::move(info.buffer);
  }

  DataStorage& storage_;

  EncodingChunkLengthSequence::encoder_type length_encoder_{};
  EncodingChunkIDSequence::encoder_type id_encoder_{};

  BareBones::Vector<SeriesToLoadInfo> series_to_load_infos_{};
  BareBones::Vector<SeriesToLoadInfo>::iterator infos_it_{};

  decltype(storage_.unloaded_snapshots_sizes.begin()) sizes_it_ = storage_.unloaded_snapshots_sizes.begin();
};
}  // namespace series_data::unloading