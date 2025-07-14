#pragma once

#include "bare_bones/bitset.h"
#include "query.h"
#include "series_data/concepts.h"
#include "series_data/data_storage.h"
#include "series_data/decoder.h"

namespace series_data::querier {

class Querier {
 public:
  explicit Querier(DataStorage& storage) : storage_(storage) {}

  template <typename Query>
  [[nodiscard]] PROMPP_ALWAYS_INLINE const QueriedChunkList& query(const Query& query) noexcept {
    chunks_.clear();

    for (auto& ls_id : query.label_set_ids) {
      query_chunks(ls_id, query.time_interval);
    }

    for (const auto& q_chunk : chunks_) {
      storage_.queried_series_bitmap.set_atomic(q_chunk.ls_id);
      if (storage_.unloaded_series_bitmap.is_set(q_chunk.ls_id)) {
        series_to_load_.set(q_chunk.ls_id);
      }
    }

    return chunks_;
  }

  bool need_loading() const noexcept { return series_to_load_.popcount() != 0; }
  const BareBones::Bitset& get_series_to_load() const noexcept { return series_to_load_; }

 private:
  using ChunkType = chunk::DataChunk::Type;

  DataStorage& storage_;
  QueriedChunkList chunks_;
  BareBones::Bitset series_to_load_;

  PROMPP_ALWAYS_INLINE void query_chunks(PromPP::Primitives::LabelSetID ls_id, const PromPP::Primitives::TimeInterval& time_interval) noexcept {
    query_finalized_chunks(ls_id, time_interval);
    query_opened_chunks(ls_id, time_interval);
  }

  void query_finalized_chunks(PromPP::Primitives::LabelSetID ls_id, const PromPP::Primitives::TimeInterval& time_interval) noexcept {
    if (const auto it = storage_.finalized_chunks.find(ls_id); it != storage_.finalized_chunks.end()) {
      uint32_t finalized_chunk_index = 0;
      auto& finalized_chunks = it->second;
      for (auto chunk_it = finalized_chunks.begin(); chunk_it != finalized_chunks.end(); ++chunk_it, ++finalized_chunk_index) {
        const auto chunk_start_timestamp_ms = Decoder::get_chunk_first_timestamp<ChunkType::kFinalized>(storage_, *chunk_it);
        if (chunk_start_timestamp_ms > time_interval.max) {
          return;
        }

        if (time_interval.intersect(
                {.min = chunk_start_timestamp_ms, .max = Decoder::get_finalized_chunk_last_timestamp(storage_, ls_id, chunk_it, finalized_chunks.end())})) {
          chunks_.emplace_back(ls_id, finalized_chunk_index);
        }
      }
    }
  }

  void query_opened_chunks(PromPP::Primitives::LabelSetID ls_id, const PromPP::Primitives::TimeInterval& time_interval) noexcept {
    if (storage_.open_chunks.size() > ls_id) {
      if (auto& open_chunk = storage_.open_chunks[ls_id]; !open_chunk.is_empty()) {
        const auto chunk_start_timestamp_ms = Decoder::get_chunk_first_timestamp<ChunkType::kOpen>(storage_, open_chunk);
        if (chunk_start_timestamp_ms > time_interval.max) {
          return;
        }

        if (time_interval.intersect({.min = chunk_start_timestamp_ms, .max = Decoder::get_open_chunk_last_timestamp(storage_, open_chunk)})) {
          chunks_.emplace_back(ls_id);
        }
      }
    }
  }
};

}  // namespace series_data::querier

static_assert(series_data::LoadableQuerierInterface<series_data::querier::Querier>);