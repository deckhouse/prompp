#pragma once
#include "bare_bones/memory.h"
#include "series_data/chunk/serialized_chunk.h"
#include "series_data/data_storage.h"
#include "series_data/decoder/universal_decode_iterator.h"
#include "series_data/querier/query.h"

namespace series_data::serialization {
class SerializedData {
 public:
  explicit SerializedData(const DataStorage& storage, const querier::QueriedChunkList& queried_chunks) noexcept { serialize_internal(storage, queried_chunks); }
  explicit SerializedData(const DataStorage& storage) noexcept { serialize_internal(storage, storage.chunks()); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE chunk::SerializedChunkSpan get_chunks() const noexcept { return {chunks_.data(), chunks_.size()}; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE std::span<const unsigned char> get_buffer() const noexcept {
    return {bytes_buffer_.control_block().data, bytes_buffer_.size()};
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t allocated_memory() const noexcept { return chunks_.allocated_memory() + bytes_buffer_.allocated_memory(); }

  [[nodiscard]] decoder::UniversalDecodeIterator create_decode_iterator(const chunk::SerializedChunk& chunk) const noexcept {
    decoder::UniversalDecodeIterator iterator(std::in_place_type<decoder::ConstantDecodeIterator>, 0, BareBones::BitSequenceReader(nullptr, 0), 0, false);
    std::span<const uint8_t> buffer{bytes_buffer_.control_block().data, bytes_buffer_.size()};
    Decoder::create_decode_iterator(buffer, chunk, [&iterator]<typename Iterator>(Iterator&& begin, auto&&) {
      iterator = decoder::UniversalDecodeIterator{std::in_place_type<Iterator>, std::forward<Iterator>(begin)};
    });
    return iterator;
  }

 private:
  struct TimestampStreamsData {
    using TimestampId = uint32_t;
    using Offset = uint32_t;

    static constexpr Offset kInvalidOffset = std::numeric_limits<Offset>::max();

    phmap::flat_hash_map<TimestampId, Offset> stream_offsets;
    phmap::flat_hash_map<TimestampId, Offset> finalized_stream_offsets;
  };

  template <class ChunkList>
  void serialize_internal(const DataStorage& storage, const ChunkList& chunks) noexcept {
    const auto& kReservedBytesForReader = encoder::CompactBitSequence::reserved_bytes_for_reader();

    const uint32_t chunk_count = get_chunk_count(chunks);

    chunks_.reserve(chunk_count);

    uint32_t data_size = 0;

    TimestampStreamsData timestamp_streams_data;
    for (auto& chunk_data : chunks) {
      using enum chunk::DataChunk::Type;

      if (chunk_data.is_open()) [[likely]] {
        if (const auto& chunk = get_chunk<kOpen>(storage, chunk_data); !chunk.is_empty()) [[likely]] {
          fill_serialized_chunk<kOpen>(storage, chunk, chunks_.emplace_back(chunk_data.series_id()), timestamp_streams_data, data_size, bytes_buffer_);
        }
      } else {
        fill_serialized_chunk<kFinalized>(storage, get_chunk<kFinalized>(storage, chunk_data), chunks_.emplace_back(chunk_data.series_id()),
                                          timestamp_streams_data, data_size, bytes_buffer_);
      }
    }

    bytes_buffer_.grow_to_fit_at_least(data_size + kReservedBytesForReader.size());
    std::memcpy(bytes_buffer_.control_block().data + data_size, kReservedBytesForReader.data(), kReservedBytesForReader.size());
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
  void fill_serialized_chunk(const DataStorage& storage,
                             const chunk::DataChunk& chunk,
                             chunk::SerializedChunk& serialized_chunk,
                             TimestampStreamsData& timestamp_streams_data,
                             uint32_t& data_size,
                             BareBones::Memory<BareBones::MemoryControlBlock, unsigned char>& buffer) const noexcept {
    using enum EncodingType;

    serialized_chunk.encoding_state = chunk.encoding_state;

    if (chunk.encoding_state.encoding_type != kGorilla) [[likely]] {
      fill_timestamp_stream_offset<chunk_type>(storage, timestamp_streams_data, chunk.timestamp_encoder_state_id, serialized_chunk, data_size, buffer);
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
        std::memcpy(buffer.control_block().data + data_size, &storage.variant_encoders[chunk.encoder.external_index].double_constant,
                    sizeof(encoder::value::DoubleConstantEncoder));
        data_size += sizeof(encoder::value::DoubleConstantEncoder);
        break;
      }

      case kTwoDoubleConstant: {
        serialized_chunk.set_offset(data_size);
        buffer.grow_to_fit_at_least(data_size + sizeof(encoder::value::TwoDoubleConstantEncoder));
        std::memcpy(buffer.control_block().data + data_size, &storage.variant_encoders[chunk.encoder.external_index].two_double_constant,
                    sizeof(encoder::value::TwoDoubleConstantEncoder));
        data_size += sizeof(encoder::value::TwoDoubleConstantEncoder);
        break;
      }

      case kAscInteger: {
        serialized_chunk.set_offset(data_size);
        write_compact_bit_sequence(storage.get_asc_integer_stream<chunk_type>(chunk.encoder.external_index), data_size, buffer);
        break;
      }

      case kAscIntegerThenValuesGorilla: {
        serialized_chunk.set_offset(data_size);
        write_compact_bit_sequence(storage.get_asc_integer_then_values_gorilla_stream<chunk_type>(chunk.encoder.external_index), data_size, buffer);
        break;
      }

      case kValuesGorilla: {
        serialized_chunk.set_offset(data_size);
        write_compact_bit_sequence(storage.get_values_gorilla_stream<chunk_type>(chunk.encoder.external_index), data_size, buffer);
        break;
      }

      case kGorilla: {
        serialized_chunk.set_offset(data_size);
        write_compact_bit_sequence(storage.get_gorilla_encoder_stream<chunk_type>(chunk.encoder.external_index), data_size, buffer);
        break;
      }

      default: {
        assert(chunk.encoding_state.encoding_type != kUnknown);
      }
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] static const chunk::DataChunk& get_chunk(const DataStorage& storage, const querier::QueriedChunk& queried_chunk) noexcept {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      return storage.open_chunks[queried_chunk.series_id()];
    } else {
      auto finalized_chunk_it = storage.finalized_chunks.find(queried_chunk.series_id())->second.begin();
      std::advance(finalized_chunk_it, queried_chunk.finalized_chunk_id);
      return *finalized_chunk_it;
    }
  }

  template <chunk::DataChunk::Type>
  [[nodiscard]] static const chunk::DataChunk& get_chunk(const DataStorage&, const DataStorage::SeriesChunkIterator::Data& chunk) noexcept {
    return chunk.chunk();
  }

  template <chunk::DataChunk::Type chunk_type>
  void fill_timestamp_stream_offset(const DataStorage& storage,
                                    TimestampStreamsData& timestamp_streams_data,
                                    encoder::timestamp::State::Id timestamp_stream_id,
                                    chunk::SerializedChunk& serialized_chunk,
                                    uint32_t& data_size,
                                    BareBones::Memory<BareBones::MemoryControlBlock, unsigned char>& buffer) const noexcept {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      if (const auto it = timestamp_streams_data.stream_offsets.find(timestamp_stream_id); it == timestamp_streams_data.stream_offsets.end()) [[unlikely]] {
        timestamp_streams_data.stream_offsets.emplace(timestamp_stream_id, data_size);
        serialized_chunk.timestamps_offset = data_size;
        write_compact_bit_sequence(storage.get_timestamp_stream<chunk_type>(timestamp_stream_id).stream, data_size, buffer);
      } else {
        serialized_chunk.timestamps_offset = it->second;
      }
    } else {
      if (const auto it = timestamp_streams_data.finalized_stream_offsets.find(timestamp_stream_id);
          it == timestamp_streams_data.finalized_stream_offsets.end()) [[unlikely]] {
        timestamp_streams_data.finalized_stream_offsets.emplace(timestamp_stream_id, data_size);
        serialized_chunk.timestamps_offset = data_size;
        write_compact_bit_sequence(storage.get_timestamp_stream<chunk_type>(timestamp_stream_id).stream, data_size, buffer);
      } else {
        serialized_chunk.timestamps_offset = it->second;
      }
    }
  }

  template <class CompactBitSequence>
  static void write_compact_bit_sequence(const CompactBitSequence& bit_sequence,
                                         uint32_t& data_size,
                                         BareBones::Memory<BareBones::MemoryControlBlock, unsigned char>& buffer) {
    const auto bytes_count = bit_sequence.size_in_bytes();
    buffer.grow_to_fit_at_least(data_size + bytes_count);
    std::memcpy(buffer.control_block().data + data_size, bit_sequence.raw_bytes(), bytes_count);
    data_size += bytes_count;
  }

  BareBones::Vector<chunk::SerializedChunk> chunks_;
  BareBones::Memory<BareBones::MemoryControlBlock, unsigned char> bytes_buffer_;
};
}  // namespace series_data::serialization