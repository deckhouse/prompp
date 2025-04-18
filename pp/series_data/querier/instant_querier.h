#pragma once

#include "primitives/primitives.h"
#include "series_data/data_storage.h"
#include "series_data/decoder.h"
#include "series_data/encoder/sample.h"

namespace series_data {
class InstantQuerier {
  using Timestamp = PromPP::Primitives::Timestamp;
  using LabelSetID = PromPP::Primitives::LabelSetID;
  using Sample = encoder::Sample;
  using ChunkType = chunk::DataChunk::Type;

public:
  PROMPP_ALWAYS_INLINE static void query_sample(Sample& sample, const DataStorage& storage, LabelSetID ls_id, const Timestamp& timestamp) noexcept {
    if (storage.open_chunks.size() <= ls_id) [[unlikely]] { return; }
    if (const auto series_last_ts = Decoder::get_series_max_timestamp(storage, ls_id); timestamp >= series_last_ts) {
      sample = {.timestamp = series_last_ts, .value = Decoder::get_open_chunk_last_value(storage, storage.open_chunks[ls_id])};
    } else if (const auto series_first_ts = Decoder::get_series_min_timestamp(storage, ls_id); timestamp >= series_first_ts) {
      check_inside_series(sample, storage, ls_id, timestamp);
    }
  }

private:
  static bool check_inside_series(Sample& sample, const DataStorage& storage, LabelSetID ls_id, const Timestamp& timestamp) noexcept {
    for (const auto& chunk_data : DataStorage::SeriesChunks(&storage, ls_id)) {
      if (Decoder::get_chunk_time_interval(chunk_data).contains(timestamp)) {
        Decoder::create_decode_iterator(chunk_data, [&](auto&& begin, auto&& end) PROMPP_LAMBDA_INLINE {
          for (auto sample_it = begin; sample_it != end && sample_it->timestamp <= timestamp; ++sample_it) {
            sample = *sample_it;
          }
        });
        return true;
      }
    }
    return false;
  }
};
} // namespace series_data
