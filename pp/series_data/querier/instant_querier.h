#pragma once

#include "primitives/primitives.h"
#include "series_data/data_storage.h"
#include "series_data/decoder.h"
#include "series_data/encoder/sample.h"

namespace series_data {
class InstantQuerier {
  using TimeInterval = PromPP::Primitives::TimeInterval;
  using LabelSetID = PromPP::Primitives::LabelSetID;
  using Sample = encoder::Sample;
  using ChunkType = chunk::DataChunk::Type;

public:
  static void query_sample(Sample& sample, const DataStorage& storage, LabelSetID ls_id, const TimeInterval& time_interval) noexcept {
    bool const is_found = check_in_open_chunk(sample, storage, ls_id, time_interval);
    if (!is_found) {
      check_in_finalized_chunks(sample, storage, ls_id, time_interval);
    }
  }

private:
  static bool check_in_open_chunk(Sample& sample, const DataStorage& storage, LabelSetID ls_id, const TimeInterval& time_interval) noexcept {
    if (storage.open_chunks.size() > ls_id) {
      if (auto& open_chunk = storage.open_chunks[ls_id]; !open_chunk.is_empty()) {
        if (const auto chunk_last_timestamp_ms = Decoder::get_open_chunk_last_timestamp(storage, open_chunk);
          time_interval.min <= chunk_last_timestamp_ms && chunk_last_timestamp_ms <= time_interval.max) {
          sample = {.timestamp = chunk_last_timestamp_ms, .value = Decoder::get_open_chunk_last_value(storage, open_chunk)};
          return true;
        }
        if (const auto chunk_first_timestamp_ms = Decoder::get_chunk_first_timestamp<ChunkType::kOpen>(storage, open_chunk);
          chunk_first_timestamp_ms <= time_interval.max) {
          const auto sample_list = Decoder::decode_chunk<ChunkType::kOpen>(storage, open_chunk);
          auto sample_it = std::ranges::upper_bound(sample_list, time_interval.max, {}, &Sample::timestamp);

          assert(sample_it != sample_list.begin());

          if ((--sample_it)->timestamp >= time_interval.min) {
            sample = *sample_it;
          }
          return true;
        }
      }
    }
    return false;
  }

  static bool check_in_finalized_chunks(Sample& sample, const DataStorage& storage, LabelSetID ls_id, const TimeInterval& time_interval) noexcept {
    if (const auto it = storage.finalized_chunks.find(ls_id); it != storage.finalized_chunks.end()) {
      auto& finalized_chunks = it->second;
      for (auto chunk_it = finalized_chunks.begin(); chunk_it != finalized_chunks.end(); ++chunk_it) {
        const auto chunk_first_timestamp_ms = Decoder::get_chunk_first_timestamp<ChunkType::kFinalized>(storage, *chunk_it);
        const auto chunk_last_timestamp_ms = Decoder::get_finalized_chunk_last_timestamp(storage, ls_id, chunk_it, finalized_chunks.end());
        if (chunk_first_timestamp_ms <= time_interval.max && time_interval.max <= chunk_last_timestamp_ms) {
          const auto sample_list = Decoder::decode_chunk<ChunkType::kFinalized>(storage, *chunk_it);
          auto sample_it = std::ranges::upper_bound(sample_list, time_interval.max, {}, &Sample::timestamp);

          assert(sample_it != sample_list.begin());

          if ((--sample_it)->timestamp >= time_interval.min) {
            sample = *sample_it;
          }
          return true;
        }
      }
    }
    return false;
  }
};
} // namespace series_data
