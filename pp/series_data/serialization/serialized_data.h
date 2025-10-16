#pragma once
#include "bare_bones/memory.h"
#include "series_data/chunk/serialized_chunk.h"
#include "series_data/data_storage.h"
#include "series_data/decoder.h"
#include "series_data/decoder/universal_decode_iterator.h"
#include "series_data/querier/query.h"

namespace series_data::serialization {

struct SerializedData {
  using Memory = BareBones::Memory<BareBones::MemoryControlBlockWithItemCount, unsigned char>;

  BareBones::Vector<chunk::SerializedChunk> chunks;
  Memory bytes_buffer;

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t allocated_memory() const noexcept { return chunks.allocated_memory() + bytes_buffer.allocated_memory(); }
};

class DataSerializer {
 public:
  explicit DataSerializer(const DataStorage& storage) : storage_(storage) {}

  SerializedData serialize(const querier::QueriedChunkList& queried_chunks) noexcept { return serialize_internal(queried_chunks); }
  SerializedData serialize() noexcept { return serialize_internal(storage_.chunks()); }

 private:
  struct TimestampStreamsData {
    using TimestampId = uint32_t;
    using Offset = uint32_t;

    static constexpr Offset kInvalidOffset = std::numeric_limits<Offset>::max();

    phmap::flat_hash_map<TimestampId, Offset> stream_offsets;
    phmap::flat_hash_map<TimestampId, Offset> finalized_stream_offsets;
  };

  template <class ChunkList>
  SerializedData serialize_internal(const ChunkList& chunks) noexcept {
    const auto& kReservedBytesForReader = encoder::CompactBitSequence::reserved_bytes_for_reader();

    SerializedData serialized_data;
    serialized_data.chunks.reserve(get_chunk_count(chunks));

    TimestampStreamsData timestamp_streams_data;
    for (auto& chunk_data : chunks) {
      using enum chunk::DataChunk::Type;

      if (chunk_data.is_open()) [[likely]] {
        if (const auto& chunk = get_chunk<kOpen>(chunk_data); !chunk.is_empty()) [[likely]] {
          fill_serialized_chunk<kOpen>(chunk, serialized_data.chunks.emplace_back(chunk_data.series_id()), timestamp_streams_data,
                                       serialized_data.bytes_buffer);
        }
      } else {
        fill_serialized_chunk<kFinalized>(get_chunk<kFinalized>(chunk_data), serialized_data.chunks.emplace_back(chunk_data.series_id()),
                                          timestamp_streams_data, serialized_data.bytes_buffer);
      }
    }

    serialized_data.bytes_buffer.grow_to_fit_at_least(serialized_data.bytes_buffer.control_block().items_count + kReservedBytesForReader.size());
    std::memcpy(serialized_data.bytes_buffer + serialized_data.bytes_buffer.control_block().items_count, kReservedBytesForReader.data(),
                kReservedBytesForReader.size());

    return serialized_data;
  }

  template <class ChunkList>
  PROMPP_ALWAYS_INLINE static uint32_t get_chunk_count(const ChunkList& chunks) noexcept {
    if constexpr (std::is_same_v<ChunkList, DataStorage::Chunks>) {
      return chunks.non_empty_chunk_count();
    } else {
      return chunks.size();
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  void fill_serialized_chunk(const chunk::DataChunk& chunk,
                             chunk::SerializedChunk& serialized_chunk,
                             TimestampStreamsData& timestamp_streams_data,
                             SerializedData::Memory& buffer) noexcept {
    using enum EncodingType;

    serialized_chunk.encoding_state = chunk.encoding_state;

    uint32_t& data_size = buffer.control_block().items_count;

    if (chunk.encoding_state.encoding_type != kGorilla) [[likely]] {
      fill_timestamp_stream_offset<chunk_type>(storage_, timestamp_streams_data, chunk.timestamp_encoder_state_id, serialized_chunk, buffer);
    }

    switch (chunk.encoding_state.encoding_type) {
      case kUint32Constant: {
        serialized_chunk.store_value_in_offset(chunk.encoder.uint32_constant);
        break;
      }

      case kFloat32Constant: {
        serialized_chunk.store_value_in_offset(chunk.encoder.float32_constant);
        break;
      }

      case kDoubleConstant: {
        serialized_chunk.set_offset(data_size);
        buffer.grow_to_fit_at_least(data_size + sizeof(encoder::value::DoubleConstantEncoder));
        std::memcpy(buffer + data_size, &storage_.variant_encoders[chunk.encoder.external_index].double_constant,
                    sizeof(encoder::value::DoubleConstantEncoder));
        data_size += sizeof(encoder::value::DoubleConstantEncoder);
        break;
      }

      case kTwoDoubleConstant: {
        serialized_chunk.set_offset(data_size);
        buffer.grow_to_fit_at_least(data_size + sizeof(encoder::value::TwoDoubleConstantEncoder));
        std::memcpy(buffer + data_size, &storage_.variant_encoders[chunk.encoder.external_index].two_double_constant,
                    sizeof(encoder::value::TwoDoubleConstantEncoder));
        data_size += sizeof(encoder::value::TwoDoubleConstantEncoder);
        break;
      }

      case kAscInteger: {
        serialized_chunk.set_offset(data_size);
        write_compact_bit_sequence(storage_.get_asc_integer_stream<chunk_type>(chunk.encoder.external_index), buffer);
        break;
      }

      case kAscIntegerThenValuesGorilla: {
        serialized_chunk.set_offset(data_size);
        write_compact_bit_sequence(storage_.get_asc_integer_then_values_gorilla_stream<chunk_type>(chunk.encoder.external_index), buffer);
        break;
      }

      case kValuesGorilla: {
        serialized_chunk.set_offset(data_size);
        write_compact_bit_sequence(storage_.get_values_gorilla_stream<chunk_type>(chunk.encoder.external_index), buffer);
        break;
      }

      case kGorilla: {
        serialized_chunk.set_offset(data_size);
        write_compact_bit_sequence(storage_.get_gorilla_encoder_stream<chunk_type>(chunk.encoder.external_index), buffer);
        break;
      }

      default: {
        assert(chunk.encoding_state.encoding_type != kUnknown);
      }
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] const chunk::DataChunk& get_chunk(const querier::QueriedChunk& queried_chunk) const noexcept {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      return storage_.open_chunks[queried_chunk.series_id()];
    } else {
      auto finalized_chunk_it = storage_.finalized_chunks.find(queried_chunk.series_id())->second.begin();
      std::advance(finalized_chunk_it, queried_chunk.finalized_chunk_id);
      return *finalized_chunk_it;
    }
  }

  template <chunk::DataChunk::Type>
  [[nodiscard]] static const chunk::DataChunk& get_chunk(const DataStorage::SeriesChunkIterator::Data& chunk) noexcept {
    return chunk.chunk();
  }

  template <chunk::DataChunk::Type chunk_type>
  static void fill_timestamp_stream_offset(const DataStorage& storage,
                                           TimestampStreamsData& timestamp_streams_data,
                                           encoder::timestamp::State::Id timestamp_stream_id,
                                           chunk::SerializedChunk& serialized_chunk,
                                           SerializedData::Memory& buffer) noexcept {
    uint32_t data_size = buffer.control_block().items_count;
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      if (const auto it = timestamp_streams_data.stream_offsets.find(timestamp_stream_id); it == timestamp_streams_data.stream_offsets.end()) [[unlikely]] {
        timestamp_streams_data.stream_offsets.emplace(timestamp_stream_id, data_size);
        serialized_chunk.timestamps_offset = data_size;
        write_compact_bit_sequence(storage.get_timestamp_stream<chunk_type>(timestamp_stream_id).stream, buffer);
      } else {
        serialized_chunk.timestamps_offset = it->second;
      }
    } else {
      if (const auto it = timestamp_streams_data.finalized_stream_offsets.find(timestamp_stream_id);
          it == timestamp_streams_data.finalized_stream_offsets.end()) [[unlikely]] {
        timestamp_streams_data.finalized_stream_offsets.emplace(timestamp_stream_id, data_size);
        serialized_chunk.timestamps_offset = data_size;
        write_compact_bit_sequence(storage.get_timestamp_stream<chunk_type>(timestamp_stream_id).stream, buffer);
      } else {
        serialized_chunk.timestamps_offset = it->second;
      }
    }
  }

  template <class CompactBitSequence>
  static void write_compact_bit_sequence(const CompactBitSequence& bit_sequence, SerializedData::Memory& buffer) noexcept {
    const auto bytes_count = bit_sequence.size_in_bytes();
    uint32_t& data_size = buffer.control_block().items_count;
    buffer.grow_to_fit_at_least(data_size + bytes_count);
    std::memcpy(buffer + data_size, bit_sequence.raw_bytes(), bytes_count);
    data_size += bytes_count;
  }

  const DataStorage& storage_;
};

class SerializedDataView {
 public:
  static constexpr uint32_t kNoMoreSeries = std::numeric_limits<uint32_t>::max();

  class SeriesIterator {
   public:
    using iterator_category = std::forward_iterator_tag;
    using value_type = encoder::Sample;
    using difference_type = ptrdiff_t;
    using pointer = value_type*;
    using reference = value_type&;

    SeriesIterator(const SerializedData::Memory& buffer, chunk::SerializedChunkSpan chunks, uint32_t chunk_id)
        : decode_iter_(std::in_place_type<decoder::ConstantDecodeIterator>, 0, BareBones::BitSequenceReader(nullptr, 0), 0, false),
          chunk_iter_(chunks.begin() + chunk_id),
          series_id_(chunk_iter_->label_set_id),
          buffer_(buffer.control_block().data, buffer.size()),
          chunks_(chunks) {
      Decoder::create_decode_iterator(buffer_, *chunk_iter_, [&]<typename Iterator>(Iterator&& begin, auto&&) {
        decode_iter_ = decoder::UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)};
      });
    }

    [[nodiscard]] PROMPP_ALWAYS_INLINE const encoder::Sample& operator*() const noexcept { return *decode_iter_; }
    [[nodiscard]] PROMPP_ALWAYS_INLINE const encoder::Sample* operator->() const noexcept { return decode_iter_.operator->(); }

    PROMPP_ALWAYS_INLINE SeriesIterator& operator++() noexcept {
      ++decode_iter_;
      if (decode_iter_ == decoder::DecodeIteratorSentinel{}) [[unlikely]] {
        if (std::next(chunk_iter_) != chunks_.end() && series_id_ == std::next(chunk_iter_)->label_set_id) {
          ++chunk_iter_;
          Decoder::create_decode_iterator(buffer_, *chunk_iter_, [&]<typename Iterator>(Iterator&& begin, auto&&) {
            decode_iter_ = decoder::UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)};
          });
        }
      }
      return *this;
    }

    PROMPP_ALWAYS_INLINE SeriesIterator operator++(int) noexcept {
      const auto it = *this;
      ++*this;
      return it;
    }

    PROMPP_ALWAYS_INLINE bool operator==(const decoder::DecodeIteratorSentinel&) const noexcept {
      return (decode_iter_ == decoder::DecodeIteratorSentinel{}) &&
             (std::next(chunk_iter_) == chunks_.end() || series_id_ != std::next(chunk_iter_)->label_set_id);
    }

   private:
    decoder::UniversalDecodeIterator decode_iter_;
    chunk::SerializedChunkSpan::const_iterator chunk_iter_;
    uint32_t series_id_;

    std::span<const unsigned char> buffer_;
    chunk::SerializedChunkSpan chunks_;
  };

  explicit SerializedDataView(const SerializedData& serialized_data) : data_(serialized_data), series_index_{kNoMoreSeries} {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE chunk::SerializedChunkSpan get_chunks_view() const noexcept { return {data_.chunks.data(), data_.chunks.size()}; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE std::span<const unsigned char> get_buffer_view() const noexcept {
    return {data_.bytes_buffer.control_block().data, data_.bytes_buffer.size()};
  }

  [[nodiscard]] uint32_t next_series() noexcept {
    const auto& chunks = data_.chunks;
    if (series_index_ == kNoMoreSeries) [[unlikely]] {
      if (chunks.empty()) [[unlikely]] {
        return kNoMoreSeries;
      }
      series_index_ = 0;
      return chunks[0].label_set_id;
    }

    if (series_index_ == chunks.size()) [[unlikely]] {
      return kNoMoreSeries;
    }

    const uint32_t current_series_id = chunks[series_index_].label_set_id;
    do {
      ++series_index_;
    } while (series_index_ < chunks.size() && chunks[series_index_].label_set_id == current_series_id);

    if (series_index_ == chunks.size()) [[unlikely]] {
      return kNoMoreSeries;
    }

    return chunks[series_index_].label_set_id;
  }

  [[nodiscard]] SeriesIterator create_current_series_iterator() const noexcept { return {data_.bytes_buffer, get_chunks_view(), series_index_}; }

 private:
  const SerializedData& data_;
  uint32_t series_index_;
};
}  // namespace series_data::serialization