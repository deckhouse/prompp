#pragma once

#include "chunk/outdated_chunk.h"
#include "common.h"
#include "data_storage.h"
#include "decoder/asc_integer_values_gorilla.h"
#include "decoder/constant.h"
#include "decoder/gorilla.h"
#include "decoder/two_double_constant.h"
#include "decoder/values_gorilla.h"
#include "primitives/primitives.h"

namespace series_data {

class Decoder {
 public:
  template <chunk::DataChunk::Type chunk_type, class Callback>
  static void decode_chunk(const DataStorage& storage, const chunk::DataChunk& chunk, Callback&& callback) noexcept {
    using enum EncodingType;
    using decoder::DecodeIteratorSentinel;

    switch (chunk.encoding_state.encoding_type) {
      case kUint32Constant: {
        std::ranges::all_of(create_decode_iterator<kUint32Constant, chunk_type>(storage, chunk), DecodeIteratorSentinel{}, std::forward<Callback>(callback));
        break;
      }

      case kFloat32Constant: {
        std::ranges::all_of(create_decode_iterator<kFloat32Constant, chunk_type>(storage, chunk), DecodeIteratorSentinel{}, std::forward<Callback>(callback));
        break;
      }

      case kDoubleConstant: {
        std::ranges::all_of(create_decode_iterator<kDoubleConstant, chunk_type>(storage, chunk), DecodeIteratorSentinel{}, std::forward<Callback>(callback));
        break;
      }

      case kTwoDoubleConstant: {
        std::ranges::all_of(create_decode_iterator<kTwoDoubleConstant, chunk_type>(storage, chunk), DecodeIteratorSentinel{}, std::forward<Callback>(callback));
        break;
      }

      case kAscIntegerValuesGorilla: {
        std::ranges::all_of(create_decode_iterator<kAscIntegerValuesGorilla, chunk_type>(storage, chunk), DecodeIteratorSentinel{},
                            std::forward<Callback>(callback));
        break;
      }

      case kValuesGorilla: {
        std::ranges::all_of(create_decode_iterator<kValuesGorilla, chunk_type>(storage, chunk), DecodeIteratorSentinel{}, std::forward<Callback>(callback));
        break;
      }

      case kGorilla: {
        std::ranges::all_of(create_decode_iterator<kGorilla, chunk_type>(storage, chunk), DecodeIteratorSentinel{}, std::forward<Callback>(callback));
        break;
      }

      case kUnknown: {
        assert(chunk.encoding_state.encoding_type != kUnknown);
        break;
      }
    }
  }

  template <class Callback>
  static void decode_chunk(const DataStorage& storage, const chunk::DataChunk& chunk, chunk::DataChunk::Type chunk_type, Callback&& callback) noexcept {
    if (chunk_type == chunk::DataChunk::Type::kOpen) {
      decode_chunk<chunk::DataChunk::Type::kOpen>(storage, chunk, std::forward<Callback>(callback));
    } else {
      decode_chunk<chunk::DataChunk::Type::kFinalized>(storage, chunk, std::forward<Callback>(callback));
    }
  }

  template <class Callback>
  PROMPP_ALWAYS_INLINE static void decode_chunk(const DataStorage::SeriesChunkIterator::Data& chunk_data, Callback&& callback) noexcept {
    decode_chunk(*chunk_data.storage(), chunk_data.chunk(), chunk_data.chunk_type(), std::forward<Callback>(callback));
  }

  static uint8_t get_samples_count(const DataStorage& storage, const chunk::DataChunk& chunk, chunk::DataChunk::Type chunk_type) noexcept {
    using enum chunk::DataChunk::Type;

    if (chunk.encoding_state.encoding_type == EncodingType::kGorilla) [[unlikely]] {
      return encoder::BitSequenceWithItemsCount::count(chunk_type == kOpen ? storage.get_gorilla_encoder_stream<kOpen>(chunk.encoder.external_index)
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

  static encoder::SampleList decode_outdated_chunk(const chunk::OutdatedChunk& chunk) {
    encoder::SampleList result;
    const auto& stream = chunk.stream().stream;
    result.reserve(encoder::BitSequenceWithItemsCount::count(stream));
    std::ranges::copy(decoder::GorillaDecodeIterator(stream, false), decoder::DecodeIteratorSentinel{}, std::back_inserter(result));
    return result;
  }

  static BareBones::Vector<encoder::SampleList> decode_chunks(const DataStorage& storage, const chunk::FinalizedChunkList& chunks) {
    BareBones::Vector<encoder::SampleList> result;
    for (auto& chunk : chunks) {
      auto& samples = result.emplace_back();
      decode_chunk<chunk::DataChunk::Type::kFinalized>(storage, chunk, [&samples](const encoder::Sample& sample) PROMPP_LAMBDA_INLINE {
        samples.emplace_back(sample);
        return true;
      });
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
                                             storage.dynamic_encoders[chunk.encoder.external_index].double_constant.value(),
                                             chunk.encoding_state.has_last_stalenan);
    } else if constexpr (encoding_type == kTwoDoubleConstant) {
      return decoder::TwoDoubleConstantDecodeIterator(storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id),
                                                      storage.dynamic_encoders[chunk.encoder.external_index].two_double_constant,
                                                      chunk.encoding_state.has_last_stalenan);
    } else if constexpr (encoding_type == kAscIntegerValuesGorilla) {
      return decoder::AscIntegerValuesGorillaDecodeIterator(
          storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id),
          chunk_type == kOpen ? storage.dynamic_encoders[chunk.encoder.external_index].asc_integer_values_gorilla.stream().reader()
                              : storage.finalized_data_streams[chunk.encoder.external_index].reader(),
          chunk.encoding_state.has_last_stalenan);
    } else if constexpr (encoding_type == kValuesGorilla) {
      return decoder::ValuesGorillaDecodeIterator(storage.get_timestamp_stream<chunk_type>(chunk.timestamp_encoder_state_id),
                                                  chunk_type == kOpen ? storage.dynamic_encoders[chunk.encoder.external_index].values_gorilla.stream().reader()
                                                                      : storage.finalized_data_streams[chunk.encoder.external_index].reader(),
                                                  chunk.encoding_state.has_last_stalenan);
    } else if constexpr (encoding_type == kGorilla) {
      return decoder::GorillaDecodeIterator(storage.get_gorilla_encoder_stream<chunk_type>(chunk.encoder.external_index),
                                            chunk.encoding_state.has_last_stalenan);
    } else {
      static_assert(encoding_type == kUnknown);
    }
  }

  [[nodiscard]] static PROMPP_ALWAYS_INLINE int64_t get_series_min_timestamp(const DataStorage& storage, uint32_t ls_id) noexcept {
    using enum chunk::DataChunk::Type;

    if (const auto it = storage.finalized_chunks.find(ls_id); it != storage.finalized_chunks.end()) {
      return get_chunk_first_timestamp<kFinalized>(storage, it->second.front());
    } else {
      return get_chunk_first_timestamp<kOpen>(storage, storage.open_chunks[ls_id]);
    }
  }

  template <chunk::DataChunk::Type chunk_type>
  [[nodiscard]] PROMPP_ALWAYS_INLINE static int64_t get_chunk_first_timestamp(const DataStorage& storage, const chunk::DataChunk& chunk) noexcept {
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
      return get_open_chunk_last_timestamp(*chunk_data.storage(), chunk_data.chunk());
    }

    return get_finalized_chunk_last_timestamp(*chunk_data.storage(), chunk_data.series_id(), chunk_data.finalized_chunk_iterator(),
                                              chunk_data.finalized_chunk_end_iterator());
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE static double get_open_chunk_last_value(const DataStorage& storage, const chunk::DataChunk& chunk) noexcept {
    using enum EncodingType;

    switch (chunk.encoding_state.encoding_type) {
      case kUint32Constant: {
        return chunk.encoder.uint32_constant.last_value(chunk.encoding_state);
      }

      case kFloat32Constant: {
        return chunk.encoder.float32_constant.last_value(chunk.encoding_state);
      }

      case kDoubleConstant: {
        return storage.dynamic_encoders[chunk.encoder.external_index].double_constant.last_value(chunk.encoding_state);
      }

      case kTwoDoubleConstant: {
        return storage.dynamic_encoders[chunk.encoder.external_index].two_double_constant.last_value(chunk.encoding_state);
      }

      case kAscIntegerValuesGorilla: {
        return storage.dynamic_encoders[chunk.encoder.external_index].asc_integer_values_gorilla.last_value(chunk.encoding_state);
      }

      case kValuesGorilla: {
        return storage.dynamic_encoders[chunk.encoder.external_index].values_gorilla.last_value(chunk.encoding_state);
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
      return encoder::BitSequenceWithItemsCount::reader(storage.finalized_data_streams[chunk.encoder.external_index]);
    }
  }
};

}  // namespace series_data
