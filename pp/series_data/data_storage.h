#pragma once

#include "bare_bones/algorithm.h"
#include "bare_bones/preprocess.h"
#include "chunk/data_chunk.h"
#include "chunk/finalized_chunk.h"
#include "chunk/outdated_chunk.h"
#include "common.h"
#include "encoder/encoder_variant.h"
#include "encoder/gorilla.h"
#include "series_data/encoder/timestamp/encoder.h"

namespace series_data {

struct DataStorage {
  class IteratorSentinel {};

  class SeriesChunkIterator {
   public:
    class Data {
     public:
      explicit Data(const DataStorage* storage, uint32_t ls_id) : storage_(storage) {
        if (storage_->open_chunks.size() > ls_id) {
          open_chunk_ = &storage_->open_chunks[ls_id];

          if (const auto it = storage_->finalized_chunks.find(ls_id); it != storage_->finalized_chunks.end()) {
            finalized_chunk_iterator_ = it->second.begin();
            finalized_chunk_end_iterator_ = it->second.end();
          }
        }
      }

      [[nodiscard]] PROMPP_ALWAYS_INLINE const DataStorage* storage() const noexcept { return storage_; }
      [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t series_id() const noexcept { return std::distance(storage_->open_chunks.begin(), open_chunk_); }
      [[nodiscard]] PROMPP_ALWAYS_INLINE chunk::DataChunk::Type chunk_type() const noexcept {
        return finalized_chunk_iterator_ == finalized_chunk_end_iterator_ ? chunk::DataChunk::Type::kOpen : chunk::DataChunk::Type::kFinalized;
      }
      [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_open() const noexcept { return chunk_type() == chunk::DataChunk::Type::kOpen; }
      [[nodiscard]] PROMPP_ALWAYS_INLINE const chunk::DataChunk& chunk() const noexcept {
        return chunk_type() == chunk::DataChunk::Type::kOpen ? *open_chunk_ : *finalized_chunk_iterator_;
      }
      [[nodiscard]] PROMPP_ALWAYS_INLINE chunk::FinalizedChunkList::ChunksList::const_iterator finalized_chunk_iterator() const noexcept {
        return finalized_chunk_iterator_;
      }
      [[nodiscard]] PROMPP_ALWAYS_INLINE chunk::FinalizedChunkList::ChunksList::const_iterator finalized_chunk_end_iterator() const noexcept {
        return finalized_chunk_end_iterator_;
      }

     private:
      friend class SeriesChunkIterator;

      const DataStorage* storage_;
      chunk::FinalizedChunkList::ChunksList::const_iterator finalized_chunk_iterator_;
      chunk::FinalizedChunkList::ChunksList::const_iterator finalized_chunk_end_iterator_;
      const chunk::DataChunk* open_chunk_{};

      PROMPP_ALWAYS_INLINE void next_value() noexcept {
        if (finalized_chunk_iterator_ != finalized_chunk_end_iterator_) {
          ++finalized_chunk_iterator_;
          return;
        }

        open_chunk_ = nullptr;
      }

      [[nodiscard]] PROMPP_ALWAYS_INLINE bool has_value() const noexcept { return open_chunk_ != nullptr; }
    };

    using iterator_category = std::forward_iterator_tag;
    using value_type = Data;
    using difference_type = ptrdiff_t;
    using pointer = Data*;
    using reference = Data&;

    explicit SeriesChunkIterator(const DataStorage* data_storage, uint32_t ls_id) : data_(data_storage, ls_id) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE const Data& operator*() const noexcept { return data_; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE const Data* operator->() const noexcept { return &data_; }

    PROMPP_ALWAYS_INLINE SeriesChunkIterator& operator++() noexcept {
      data_.next_value();
      return *this;
    }

    PROMPP_ALWAYS_INLINE SeriesChunkIterator operator++(int) noexcept {
      const auto it = *this;
      ++*this;
      return it;
    }

    PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept { return !data_.has_value(); }

   private:
    Data data_;
  };

  class ChunkIterator {
   public:
    using iterator_category = std::forward_iterator_tag;
    using value_type = SeriesChunkIterator::Data;
    using difference_type = ptrdiff_t;
    using pointer = SeriesChunkIterator::Data*;
    using reference = SeriesChunkIterator::Data&;

    explicit ChunkIterator(const DataStorage* storage) : storage_(storage), iterator_(storage->open_chunks.begin()), series_chunk_iterator_(storage, 0U) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE const SeriesChunkIterator::Data& operator*() const noexcept { return *series_chunk_iterator_; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE const SeriesChunkIterator::Data* operator->() const noexcept { return series_chunk_iterator_.operator->(); }

    PROMPP_ALWAYS_INLINE ChunkIterator& operator++() noexcept {
      if (++series_chunk_iterator_ == IteratorSentinel{}) {
        if (++iterator_ != storage_->open_chunks.end()) {
          series_chunk_iterator_ = SeriesChunkIterator{storage_, static_cast<uint32_t>(std::distance(storage_->open_chunks.begin(), iterator_))};
        }
      }
      return *this;
    }

    PROMPP_ALWAYS_INLINE ChunkIterator operator++(int) noexcept {
      const auto it = *this;
      ++*this;
      return it;
    }

    PROMPP_ALWAYS_INLINE bool operator==(const IteratorSentinel&) const noexcept {
      return iterator_ == storage_->open_chunks.end() && series_chunk_iterator_ == IteratorSentinel{};
    }

   private:
    const DataStorage* storage_;
    BareBones::Vector<chunk::DataChunk>::const_iterator iterator_;
    SeriesChunkIterator series_chunk_iterator_;
  };

  class SeriesChunks {
   public:
    explicit SeriesChunks(const DataStorage* storage, uint32_t series_id) : storage_(storage), series_id_(series_id) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE SeriesChunkIterator begin() const noexcept { return SeriesChunkIterator{storage_, series_id_}; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

   private:
    const DataStorage* storage_;
    const uint32_t series_id_;
  };

  class Chunks {
   public:
    explicit Chunks(const DataStorage* storage) : storage_(storage) {}

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t non_empty_chunk_count() const noexcept { return non_empty_open_chunk_count() + finalized_chunk_count(); }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t non_empty_open_chunk_count() const noexcept {
      return std::ranges::count_if(storage_->open_chunks, [](const chunk::DataChunk& chunk) { return !chunk.is_empty(); });
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t finalized_chunk_count() const noexcept {
      return std::accumulate(storage_->finalized_chunks.begin(), storage_->finalized_chunks.end(), uint32_t{},
                             [](uint32_t count, const auto& chunks_pair) PROMPP_LAMBDA_INLINE { return count + chunks_pair.second.count(); });
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE ChunkIterator begin() const noexcept { return ChunkIterator{storage_}; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE static IteratorSentinel end() noexcept { return {}; }

   private:
    const DataStorage* storage_;
  };

  BareBones::Vector<chunk::DataChunk> open_chunks;
  encoder::timestamp::Encoder timestamp_encoder;

  BareBones::VectorWithHoles<encoder::EncoderVariant> variant_encoders;
  BareBones::VectorWithHoles<encoder::GorillaEncoder> gorilla_encoders;

  size_t outdated_chunks_map_allocated_memory{};
  phmap::
      flat_hash_map<uint32_t, chunk::OutdatedChunk, std::hash<uint32_t>, std::equal_to<>, BareBones::Allocator<std::pair<const uint32_t, chunk::OutdatedChunk>>>
          outdated_chunks{{}, {}, BareBones::Allocator<std::pair<const uint32_t, chunk::OutdatedChunk>>{outdated_chunks_map_allocated_memory}};

  BareBones::VectorWithHoles<encoder::RefCountableBitSequenceWithItemsCount> finalized_timestamp_streams;
  BareBones::VectorWithHoles<encoder::CompactBitSequence> finalized_data_streams;
  size_t finalized_chunks_map_allocated_memory{};
  phmap::flat_hash_map<uint32_t,
                       chunk::FinalizedChunkList,
                       std::hash<uint32_t>,
                       std::equal_to<>,
                       BareBones::Allocator<std::pair<const uint32_t, std::forward_list<chunk::DataChunk>>>>
      finalized_chunks{{}, {}, BareBones::Allocator<std::pair<const uint32_t, std::forward_list<chunk::DataChunk>>>{finalized_chunks_map_allocated_memory}};

  uint32_t samples_count{};
  uint32_t outdated_samples_count{};
  uint32_t outdated_chunks_count{};
  uint32_t merged_samples_count{};

  [[nodiscard]] PROMPP_ALWAYS_INLINE SeriesChunks chunks(uint32_t ls_id) const noexcept { return SeriesChunks{this, ls_id}; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE Chunks chunks() const noexcept { return Chunks{this}; }

  void reset_sample_counters() noexcept {
    samples_count = 0;
    outdated_samples_count = 0;
    merged_samples_count = 0;
  }

  void delete_finalized_chunk(uint32_t ls_id, const chunk::DataChunk& chunk) noexcept {
    if (const auto finalized_it = finalized_chunks.find(ls_id); finalized_it != finalized_chunks.end()) {
      erase_chunk_timestamp_and_encoder<chunk::DataChunk::Type::kFinalized>(chunk);
      finalized_it->second.erase(chunk);
      if (finalized_it->second.count() == 0) {
        finalized_chunks.erase(finalized_it);
      }
    }
  }

  void delete_open_chunk(uint32_t ls_id) noexcept {
    auto& chunk = open_chunks[ls_id];
    erase_chunk_timestamp_and_encoder<chunk::DataChunk::Type::kOpen>(chunk);
    chunk.reset();
  }

  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] PROMPP_ALWAYS_INLINE const encoder::BitSequenceWithItemsCount& get_timestamp_stream(uint32_t stream_id) const noexcept {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      if (const auto& state = timestamp_encoder.get_state(stream_id); !state.is_finalized()) [[likely]] {
        return timestamp_encoder.get_stream(stream_id);
      } else {
        return finalized_timestamp_streams[state.stream_data.finalized_stream_id].stream;
      }
    } else {
      return finalized_timestamp_streams[stream_id].stream;
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] PROMPP_ALWAYS_INLINE const encoder::CompactBitSequence& get_asc_integer_stream(uint32_t stream_id) const noexcept {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      return variant_encoders[stream_id].asc_integer.stream();
    } else {
      return finalized_data_streams[stream_id];
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] PROMPP_ALWAYS_INLINE const encoder::CompactBitSequence& get_values_gorilla_stream(uint32_t stream_id) const noexcept {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      return variant_encoders[stream_id].values_gorilla.stream();
    } else {
      return finalized_data_streams[stream_id];
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] PROMPP_ALWAYS_INLINE const encoder::CompactBitSequence& get_asc_integer_then_values_gorilla_stream(uint32_t stream_id) const noexcept {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      return variant_encoders[stream_id].asc_integer_then_values_gorilla.stream();
    } else {
      return finalized_data_streams[stream_id];
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] PROMPP_ALWAYS_INLINE const encoder::CompactBitSequence& get_gorilla_encoder_stream(uint32_t stream_id) const noexcept {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      return gorilla_encoders[stream_id].stream().stream;
    } else {
      return finalized_data_streams[stream_id];
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    using enum EncodingType;

    const size_t outdated_chunks_allocated_memory =
        BareBones::accumulate(outdated_chunks, 0, [](auto& local, const auto& p) { return local + p.second.allocated_memory(); });

    size_t encoders_memory = variant_encoders.allocated_memory() + gorilla_encoders.allocated_memory();

    for (const auto& chunk : open_chunks) {
      encoders_memory += variant_encoders[chunk.encoder.external_index].allocated_memory(chunk.encoding_state.encoding_type);
    }

    return open_chunks.allocated_memory() + encoders_memory + timestamp_encoder.allocated_memory() + finalized_timestamp_streams.allocated_memory() +
           finalized_data_streams.allocated_memory() + finalized_chunks_map_allocated_memory + outdated_chunks_map_allocated_memory +
           outdated_chunks_allocated_memory;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory(EncodingType encoding_type) const noexcept {
    if (is_variant_encoder(encoding_type)) {
      return BareBones::accumulate(open_chunks, 0ULL, [this, encoding_type](size_t allocated_memory, const auto& chunk) {
        if (chunk.encoding_state.encoding_type == encoding_type) {
          return allocated_memory + variant_encoders[chunk.encoder.external_index].allocated_memory(encoding_type);
        }

        return allocated_memory;
      });
    }
    if (encoding_type == EncodingType::kGorilla) {
      return gorilla_encoders.allocated_memory();
    }
    return 0;
  }

  ~DataStorage() {
    for (const auto& chunk : open_chunks) {
      destroy_open_chunk_encoder(chunk);
    }
  }

  void reset() noexcept {
    std::destroy_at(this);
    std::construct_at(this);
  }

 private:
  template <chunk::DataChunk::Type chunk_type>
  void erase_chunk_timestamp_and_encoder(const chunk::DataChunk& chunk) {
    if (chunk.encoding_state.encoding_type != EncodingType::kGorilla) {
      erase_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id);
    }

    erase_encoder_data<chunk_type>(chunk);
  }

  template <chunk::DataChunk::Type chunk_type>
  void erase_timestamp_stream(uint32_t stream_id) {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      timestamp_encoder.erase(stream_id);
    } else {
      if (--finalized_timestamp_streams[stream_id].reference_count == 0) {
        finalized_timestamp_streams.erase(stream_id);
      }
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  void erase_encoder_data(const chunk::DataChunk& chunk) {
    using enum EncodingType;

    if constexpr (chunk_type == chunk::DataChunk::Type::kFinalized) {
      if (is_gorilla_based_encoder(chunk.encoding_state.encoding_type)) {
        finalized_data_streams.erase(chunk.encoder.external_index);
        return;
      }
    }
    if (chunk.encoding_state.encoding_type == kGorilla) {
      gorilla_encoders.erase(chunk.encoder.external_index, kGorilla);
    } else if (is_variant_encoder(chunk.encoding_state.encoding_type)) {
      variant_encoders.erase(chunk.encoder.external_index, chunk.encoding_state.encoding_type);
    }
  }

  void destroy_open_chunk_encoder(const chunk::DataChunk& chunk) {
    if (is_variant_encoder(chunk.encoding_state.encoding_type)) {
      variant_encoders[chunk.encoder.external_index].destroy(chunk.encoding_state.encoding_type);
    }
  }
};

}  // namespace series_data
