#pragma once

#include "bare_bones/exception.h"
#include "chunk/outdated_chunk.h"
#include "chunk/serialized_chunk.h"
#include "common.h"
#include "data_storage.h"
#include "decoder/asc_integer.h"
#include "decoder/asc_integer_then_values_gorilla.h"
#include "decoder/constant.h"
#include "decoder/gorilla.h"
#include "decoder/outdated.h"
#include "decoder/two_double_constant.h"
#include "decoder/values_gorilla.h"
#include "primitives/primitives.h"

namespace series_data {

#pragma pack(push, 1)

struct SerializedCompactBitSequence {
  template <class CompactBitSequence>
  PROMPP_ALWAYS_INLINE explicit SerializedCompactBitSequence(const CompactBitSequence& bit_sequence)
      : ptr(bit_sequence.shared_memory()), size_in_bits(bit_sequence.size_in_bits()) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE std::span<const uint8_t> buffer() const noexcept { return {ptr.get(), BareBones::Bit::to_ceil_bytes(size_in_bits)}; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE BareBones::BitSequenceReader reader() const noexcept { return {ptr.get(), size_in_bits}; }

  encoder::CompactBitSequence<DataStorage::Reallocator>::SharedPtr ptr;
  uint32_t size_in_bits;
};

#pragma pack(pop)

class Decoder {
 public:
  template <chunk::DataChunk::Type chunk_type, class Callback>
  PROMPP_ALWAYS_INLINE static void decode_chunk(const DataStorage& storage, const chunk::DataChunk& chunk, Callback&& callback) noexcept {
    create_decode_iterator<chunk_type>(storage, chunk,
                                       [&callback](auto&& begin, auto&& end) { std::ranges::all_of(begin, end, std::forward<Callback>(callback)); });
  }

  static uint8_t get_samples_count(const DataStorage& storage, const chunk::DataChunk& chunk, chunk::DataChunk::Type chunk_type) noexcept {
    using enum chunk::DataChunk::Type;

    if (chunk.encoding_state.encoding_type == EncodingType::kGorilla) [[unlikely]] {
      return DataStorage::BitSequenceWithItemsCount::count(chunk_type == kOpen ? storage.get_gorilla_encoder_stream<kOpen>(chunk.encoder.external_index)
                                                                               : storage.get_gorilla_encoder_stream<kFinalized>(chunk.encoder.external_index));
    } else {
      return (chunk_type == kOpen ? storage.get_timestamp_stream<kOpen>(chunk.timestamp_encoder_state_id)
                                  : storage.get_timestamp_stream<kFinalized>(chunk.timestamp_encoder_state_id))
          .count();
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  static encoder::SampleList decode_chunk(const DataStorage& storage, const chunk::DataChunk& chunk) {
    encoder::SampleList result;
    decode_chunk<chunk_type>(storage, chunk, [&result](const encoder::Sample& sample) PROMPP_LAMBDA_INLINE {
      result.emplace_back(sample);
      return true;
    });
    return result;
  }

  static encoder::SampleList decode_outdated_chunk(const DataStorage::OutdatedChunk& chunk) {
    encoder::SampleList result;
    result.reserve(chunk.samples_count());
    std::ranges::copy(decoder::OutdatedDecodeIterator(chunk.samples_count(), chunk.stream().reader()), decoder::DecodeIteratorSentinel{},
                      std::back_inserter(result));
    return result;
  }

  static BareBones::Vector<encoder::SampleList> decode_chunks(const DataStorage& storage, const chunk::FinalizedChunkList& chunks) {
    BareBones::Vector<encoder::SampleList> result;
    for (auto& chunk : chunks) {
      result.emplace_back(decode_chunk<chunk::DataChunk::Type::kFinalized>(storage, chunk));
    }
    return result;
  }

  static BareBones::Vector<encoder::SampleList> decode_chunks(const DataStorage& storage,
                                                              const chunk::FinalizedChunkList& finalized_chunks,
                                                              const chunk::DataChunk& open_chunk) {
    auto result = decode_chunks(storage, finalized_chunks);
    auto& open_chunk_samples = result.emplace_back();
    decode_chunk<chunk::DataChunk::Type::kOpen>(storage, open_chunk, [&open_chunk_samples](const encoder::Sample& sample) PROMPP_LAMBDA_INLINE {
      open_chunk_samples.emplace_back(sample);
      return true;
    });
    return result;
  }
  template <class Callback>
  static void decode_series(const DataStorage& storage, uint32_t ls_id, Callback&& callback) {
    auto& finalized_chunks = storage.finalized_chunks;
    if (const auto finalized_chunks_it = finalized_chunks.find(ls_id); finalized_chunks_it != finalized_chunks.end()) {
      for (const auto& chunk : finalized_chunks_it->second) {
        Decoder::decode_chunk<chunk::DataChunk::Type::kFinalized>(storage, chunk, callback);
      }
    }

    Decoder::decode_chunk<chunk::DataChunk::Type::kOpen>(storage, storage.open_chunks[ls_id], callback);
  }

  template <EncodingType encoding_type, chunk::DataChunk::Type chunk_type>
  static auto create_decode_iterator(const DataStorage& storage, const chunk::DataChunk& chunk) {
    using enum EncodingType;
    using enum chunk::DataChunk::Type;

    if constexpr (encoding_type == kUint32Constant) {
      return decoder::ConstantDecodeIterator(storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id), chunk.encoder.uint32_constant.value(),
                                             chunk.encoding_state.has_last_stalenan);
    } else if constexpr (encoding_type == kFloat32Constant) {
      return decoder::ConstantDecodeIterator(storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id), chunk.encoder.float32_constant.value(),
                                             chunk.encoding_state.has_last_stalenan);
    } else if constexpr (encoding_type == kDoubleConstant) {
      return decoder::ConstantDecodeIterator(storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id),
                                             storage.variant_encoders[chunk.encoder.external_index].double_constant.value(),
                                             chunk.encoding_state.has_last_stalenan);
    } else if constexpr (encoding_type == kTwoDoubleConstant) {
      return decoder::TwoDoubleConstantDecodeIterator(storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id),
                                                      storage.variant_encoders[chunk.encoder.external_index].two_double_constant,
                                                      chunk.encoding_state.has_last_stalenan);
    } else if constexpr (encoding_type == kAscInteger) {
      return decoder::AscIntegerDecodeIterator(storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id),
                                               storage.get_asc_integer_stream<chunk_type>(chunk.encoder.external_index).reader());
    } else if constexpr (encoding_type == kAscIntegerThenValuesGorilla) {
      return decoder::AscIntegerThenValuesGorillaDecodeIterator(
          storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id),
          storage.get_asc_integer_then_values_gorilla_stream<chunk_type>(chunk.encoder.external_index).reader());
    } else if constexpr (encoding_type == kValuesGorilla) {
      return decoder::ValuesGorillaDecodeIterator(storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id),
                                                  storage.get_values_gorilla_stream<chunk_type>(chunk.encoder.external_index).reader());
    } else if constexpr (encoding_type == kGorilla) {
      return decoder::GorillaDecodeIterator(storage.get_gorilla_encoder_stream<chunk_type>(chunk.encoder.external_index));
    } else {
      static_assert(encoding_type == kUnknown);
    }
  }

  template <chunk::DataChunk::Type chunk_type, class Callback>
  static void create_decode_iterator(const DataStorage& storage, const chunk::DataChunk& chunk, Callback&& callback) {
    using enum EncodingType;
    using decoder::DecodeIteratorSentinel;

    switch (chunk.encoding_state.encoding_type) {
      case kUint32Constant: {
        std::forward<Callback>(callback)(create_decode_iterator<kUint32Constant, chunk_type>(storage, chunk), DecodeIteratorSentinel{});
        break;
      }

      case kFloat32Constant: {
        std::forward<Callback>(callback)(create_decode_iterator<kFloat32Constant, chunk_type>(storage, chunk), DecodeIteratorSentinel{});
        break;
      }

      case kDoubleConstant: {
        std::forward<Callback>(callback)(create_decode_iterator<kDoubleConstant, chunk_type>(storage, chunk), DecodeIteratorSentinel{});
        break;
      }

      case kTwoDoubleConstant: {
        std::forward<Callback>(callback)(create_decode_iterator<kTwoDoubleConstant, chunk_type>(storage, chunk), DecodeIteratorSentinel{});
        break;
      }

      case kAscInteger: {
        std::forward<Callback>(callback)(create_decode_iterator<kAscInteger, chunk_type>(storage, chunk), DecodeIteratorSentinel{});
        break;
      }

      case kAscIntegerThenValuesGorilla: {
        std::forward<Callback>(callback)(create_decode_iterator<kAscIntegerThenValuesGorilla, chunk_type>(storage, chunk), DecodeIteratorSentinel{});
        break;
      }

      case kValuesGorilla: {
        std::forward<Callback>(callback)(create_decode_iterator<kValuesGorilla, chunk_type>(storage, chunk), DecodeIteratorSentinel{});
        break;
      }

      case kGorilla: {
        std::forward<Callback>(callback)(create_decode_iterator<kGorilla, chunk_type>(storage, chunk), DecodeIteratorSentinel{});
        break;
      }

      default: {
        assert(chunk.encoding_state.encoding_type != kUnknown);
        break;
      }
    }
  }

  template <class Callback>
  static void create_decode_iterator(const DataStorage::SeriesChunkIterator::Data& chunk_data, Callback&& callback) {
    using enum chunk::DataChunk::Type;

    if (chunk_data.chunk_type() == kOpen) {
      create_decode_iterator<kOpen>(*chunk_data.storage(), chunk_data.chunk(), std::forward<Callback>(callback));
    } else {
      create_decode_iterator<kFinalized>(*chunk_data.storage(), chunk_data.chunk(), std::forward<Callback>(callback));
    }
  }

  template <class Callback>
  static auto create_decode_iterator(std::span<const uint8_t> buffer, const chunk::SerializedChunk& chunk, Callback&& callback) {
    using enum EncodingType;
    using BitSequenceWithItemsCount = DataStorage::BitSequenceWithItemsCount;
    using decoder::DecodeIteratorSentinel;

    switch (chunk.encoding_state.encoding_type) {
      case kUint32Constant: {
        const auto timestamp_bit_sequence = reinterpret_cast<const SerializedCompactBitSequence*>(buffer.data() + chunk.timestamps_offset);
        return std::forward<Callback>(callback)(decoder::ConstantDecodeIterator(BitSequenceWithItemsCount::count(timestamp_bit_sequence->ptr.get()),
                                                                                BitSequenceWithItemsCount::reader(timestamp_bit_sequence->buffer()),
                                                                                chunk.values_offset, chunk.encoding_state.has_last_stalenan),
                                                DecodeIteratorSentinel{});
      }

      case kFloat32Constant: {
        const auto timestamp_bit_sequence = reinterpret_cast<const SerializedCompactBitSequence*>(buffer.data() + chunk.timestamps_offset);
        return std::forward<Callback>(callback)(
            decoder::ConstantDecodeIterator(BitSequenceWithItemsCount::count(timestamp_bit_sequence->ptr.get()),
                                            BitSequenceWithItemsCount::reader(timestamp_bit_sequence->buffer()), std::bit_cast<float>(chunk.values_offset),
                                            chunk.encoding_state.has_last_stalenan),
            DecodeIteratorSentinel{});
      }

      case kDoubleConstant: {
        const auto timestamp_bit_sequence = reinterpret_cast<const SerializedCompactBitSequence*>(buffer.data() + chunk.timestamps_offset);
        const auto values_buffer = buffer.subspan(chunk.values_offset);
        assert(values_buffer.size() >= sizeof(double));
        return std::forward<Callback>(callback)(
            decoder::ConstantDecodeIterator(BitSequenceWithItemsCount::count(timestamp_bit_sequence->ptr.get()),
                                            BitSequenceWithItemsCount::reader(timestamp_bit_sequence->buffer()),
                                            *reinterpret_cast<const double*>(values_buffer.data()), chunk.encoding_state.has_last_stalenan),
            DecodeIteratorSentinel{});
      }

      case kTwoDoubleConstant: {
        const auto timestamp_bit_sequence = reinterpret_cast<const SerializedCompactBitSequence*>(buffer.data() + chunk.timestamps_offset);
        const auto values_buffer = buffer.subspan(chunk.values_offset);
        assert(values_buffer.size() >= sizeof(encoder::value::TwoDoubleConstantEncoder));
        return std::forward<Callback>(callback)(
            decoder::TwoDoubleConstantDecodeIterator(
                BitSequenceWithItemsCount::count(timestamp_bit_sequence->ptr.get()), BitSequenceWithItemsCount::reader(timestamp_bit_sequence->buffer()),
                *reinterpret_cast<const encoder::value::TwoDoubleConstantEncoder*>(values_buffer.data()), chunk.encoding_state.has_last_stalenan),
            DecodeIteratorSentinel{});
      }

      case kAscInteger: {
        const auto timestamp_bit_sequence = reinterpret_cast<const SerializedCompactBitSequence*>(buffer.data() + chunk.timestamps_offset);
        const auto values_bit_sequence = reinterpret_cast<const SerializedCompactBitSequence*>(buffer.data() + chunk.values_offset);
        return std::forward<Callback>(callback)(
            decoder::AscIntegerDecodeIterator(BitSequenceWithItemsCount::count(timestamp_bit_sequence->ptr.get()),
                                              BitSequenceWithItemsCount::reader(timestamp_bit_sequence->buffer()), values_bit_sequence->reader()),
            DecodeIteratorSentinel{});
      }

      case kAscIntegerThenValuesGorilla: {
        const auto timestamp_bit_sequence = reinterpret_cast<const SerializedCompactBitSequence*>(buffer.data() + chunk.timestamps_offset);
        const auto values_bit_sequence = reinterpret_cast<const SerializedCompactBitSequence*>(buffer.data() + chunk.values_offset);
        return std::forward<Callback>(callback)(decoder::AscIntegerThenValuesGorillaDecodeIterator(
                                                    BitSequenceWithItemsCount::count(timestamp_bit_sequence->ptr.get()),
                                                    BitSequenceWithItemsCount::reader(timestamp_bit_sequence->buffer()), values_bit_sequence->reader()),
                                                DecodeIteratorSentinel{});
      }

      case kValuesGorilla: {
        const auto timestamp_bit_sequence = reinterpret_cast<const SerializedCompactBitSequence*>(buffer.data() + chunk.timestamps_offset);
        const auto values_bit_sequence = reinterpret_cast<const SerializedCompactBitSequence*>(buffer.data() + chunk.values_offset);
        return std::forward<Callback>(callback)(
            decoder::ValuesGorillaDecodeIterator(BitSequenceWithItemsCount::count(timestamp_bit_sequence->ptr.get()),
                                                 BitSequenceWithItemsCount::reader(timestamp_bit_sequence->buffer()), values_bit_sequence->reader()),
            DecodeIteratorSentinel{});
      }

      case kGorilla: {
        const auto bit_sequence = reinterpret_cast<const SerializedCompactBitSequence*>(buffer.data() + chunk.values_offset);
        return std::forward<Callback>(callback)(decoder::GorillaDecodeIterator(BitSequenceWithItemsCount::count(bit_sequence->ptr.get()),
                                                                               BitSequenceWithItemsCount::reader(bit_sequence->buffer())),
                                                DecodeIteratorSentinel{});
      }

      default: {
        throw BareBones::Exception(0x152a003c6f8d23af, "invalid data storage encoder type");
      }
    }
  }

  template <class Callback>
  PROMPP_ALWAYS_INLINE static void create_decode_iterator(const chunk::SerializedChunkIterator::Data& chunk, Callback&& callback) {
    create_decode_iterator(chunk.buffer(), chunk.chunk(), std::forward<Callback>(callback));
  }

  [[nodiscard]] static PROMPP_ALWAYS_INLINE int64_t get_series_min_timestamp(const DataStorage& storage, uint32_t ls_id) noexcept {
    using enum chunk::DataChunk::Type;

    if (const auto it = storage.finalized_chunks.find(ls_id); it != storage.finalized_chunks.end()) {
      return get_chunk_first_timestamp<kFinalized>(storage, it->second.front());
    }
    return get_chunk_first_timestamp<kOpen>(storage, storage.open_chunks[ls_id]);
  }

  [[nodiscard]] static PROMPP_ALWAYS_INLINE int64_t get_series_max_timestamp(const DataStorage& storage, uint32_t ls_id) noexcept {
    using enum chunk::DataChunk::Type;

    return get_open_chunk_last_timestamp(storage, storage.open_chunks[ls_id]);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static PromPP::Primitives::TimeInterval get_series_time_interval(const DataStorage& storage, uint32_t ls_id) {
    return {.min = get_series_min_timestamp(storage, ls_id), .max = get_series_max_timestamp(storage, ls_id)};
  }

  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] PROMPP_ALWAYS_INLINE static int64_t get_chunk_first_timestamp(const DataStorage& storage, const chunk::DataChunk& chunk) noexcept {
    assert(!chunk.is_empty());
    return encoder::timestamp::TimestampDecoder::decode_first(get_stream_reader<chunk_type>(storage, chunk));
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static int64_t get_chunk_first_timestamp(const DataStorage& storage,
                                                                              const chunk::DataChunk& chunk,
                                                                              chunk::DataChunk::Type chunk_type) noexcept {
    if (chunk_type == chunk::DataChunk::Type::kOpen) {
      return get_chunk_first_timestamp<chunk::DataChunk::Type::kOpen>(storage, chunk);
    }
    return get_chunk_first_timestamp<chunk::DataChunk::Type::kFinalized>(storage, chunk);
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static int64_t get_chunk_first_timestamp(const DataStorage::SeriesChunkIterator::Data& chunk_data) noexcept {
    return get_chunk_first_timestamp(*chunk_data.storage(), chunk_data.chunk(), chunk_data.chunk_type());
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static int64_t get_open_chunk_last_timestamp(const DataStorage& storage, const chunk::DataChunk& chunk) noexcept {
    if (chunk.encoding_state.encoding_type == EncodingType::kGorilla) [[unlikely]] {
      return storage.gorilla_encoders[chunk.encoder.external_index].timestamp();
    }

    assert(!chunk.is_empty());
    return storage.timestamp_encoder.get_state(chunk.timestamp_encoder_state_id).timestamp();
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static int64_t get_finalized_chunk_last_timestamp(const DataStorage& storage,
                                                                                       uint32_t ls_id,
                                                                                       chunk::FinalizedChunkList::ChunksList::const_iterator chunk_it,
                                                                                       chunk::FinalizedChunkList::ChunksList::const_iterator end_it) noexcept {
    if (const auto next_chunk_it = std::next(chunk_it); next_chunk_it != end_it) {
      return get_chunk_first_timestamp<chunk::DataChunk::Type::kFinalized>(storage, *next_chunk_it) - 1;
    }
    return get_chunk_first_timestamp<chunk::DataChunk::Type::kOpen>(storage, storage.open_chunks[ls_id]) - 1;
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static int64_t get_chunk_last_timestamp(const DataStorage::SeriesChunkIterator::Data& chunk_data) noexcept {
    if (chunk_data.chunk_type() == chunk::DataChunk::Type::kOpen) {
      return get_chunk_last_timestamp<chunk::DataChunk::Type::kOpen>(chunk_data);
    }

    return get_chunk_last_timestamp<chunk::DataChunk::Type::kFinalized>(chunk_data);
  }

  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] PROMPP_ALWAYS_INLINE static int64_t get_chunk_last_timestamp(const DataStorage::SeriesChunkIterator::Data& chunk_data) noexcept {
    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      return get_open_chunk_last_timestamp(*chunk_data.storage(), chunk_data.chunk());
    }

    return get_finalized_chunk_last_timestamp(*chunk_data.storage(), chunk_data.series_id(), chunk_data.finalized_chunk_iterator(),
                                              chunk_data.finalized_chunk_end_iterator());
  }

  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] PROMPP_ALWAYS_INLINE static PromPP::Primitives::TimeInterval get_chunk_time_interval(const DataStorage::SeriesChunkIterator::Data& chunk_data) {
    return {.min = get_chunk_first_timestamp<chunk_type>(*chunk_data.storage(), chunk_data.chunk()), .max = get_chunk_last_timestamp<chunk_type>(chunk_data)};
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static PromPP::Primitives::TimeInterval get_chunk_time_interval(const DataStorage::SeriesChunkIterator::Data& chunk_data) {
    if (chunk_data.chunk_type() == chunk::DataChunk::Type::kOpen) {
      return get_chunk_time_interval<chunk::DataChunk::Type::kOpen>(chunk_data);
    }
    return get_chunk_time_interval<chunk::DataChunk::Type::kFinalized>(chunk_data);
  }

  [[nodiscard]]
  PROMPP_ALWAYS_INLINE static double get_open_chunk_last_value(const DataStorage& storage, const chunk::DataChunk& chunk) noexcept {
    using enum EncodingType;

    switch (chunk.encoding_state.encoding_type) {
      case kUint32Constant: {
        return chunk.encoder.uint32_constant.last_value(chunk.encoding_state);
      }

      case kFloat32Constant: {
        return chunk.encoder.float32_constant.last_value(chunk.encoding_state);
      }

      case kDoubleConstant: {
        return storage.variant_encoders[chunk.encoder.external_index].double_constant.last_value(chunk.encoding_state);
      }

      case kTwoDoubleConstant: {
        return storage.variant_encoders[chunk.encoder.external_index].two_double_constant.last_value(chunk.encoding_state);
      }

      case kAscInteger: {
        return storage.variant_encoders[chunk.encoder.external_index].asc_integer.last_value(chunk.encoding_state);
      }

      case kAscIntegerThenValuesGorilla: {
        return storage.variant_encoders[chunk.encoder.external_index].asc_integer_then_values_gorilla.last_value(chunk.encoding_state);
      }

      case kValuesGorilla: {
        return storage.variant_encoders[chunk.encoder.external_index].values_gorilla.last_value(chunk.encoding_state);
      }

      case kGorilla: {
        return storage.gorilla_encoders[chunk.encoder.external_index].last_value(chunk.encoding_state);
      }

      default: {
        assert(chunk.encoding_state.encoding_type != kUint32Constant);
        return 0.0;
      }
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static PromPP::Primitives::TimeInterval get_time_interval(const DataStorage& storage) noexcept {
    PromPP::Primitives::TimeInterval interval;

    for (auto ls_id = 0U; ls_id < storage.open_chunks.size(); ++ls_id) {
      if (auto& chunk = storage.open_chunks[ls_id]; !chunk.is_empty()) {
        interval.min = std::min(interval.min, get_series_min_timestamp(storage, ls_id));
        interval.max = std::max(interval.max, get_open_chunk_last_timestamp(storage, chunk));
      }
    }

    return interval;
  }

 private:
  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] static BareBones::BitSequenceReader get_stream_reader(const DataStorage& storage, const chunk::DataChunk& chunk) {
    if (chunk.encoding_state.encoding_type != EncodingType::kGorilla) [[likely]] {
      return storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id).reader();
    }

    if constexpr (chunk_type == chunk::DataChunk::Type::kOpen) {
      return storage.gorilla_encoders[chunk.encoder.external_index].stream().reader();
    } else {
      return DataStorage::BitSequenceWithItemsCount::reader(storage.finalized_data_streams[chunk.encoder.external_index]);
    }
  }
};

}  // namespace series_data
