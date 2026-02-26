#pragma once

#include "bare_bones/bitset.h"
#include "primitives/primitives.h"
#include "series_data/concepts.h"
#include "series_data/data_storage.h"
#include "series_data/decoder.h"
#include "series_data/encoder/sample.h"

#include <ranges>

namespace series_data {

class InstantQuerier {
  using Timestamp = PromPP::Primitives::Timestamp;
  using LabelSetID = PromPP::Primitives::LabelSetID;
  using Sample = encoder::Sample;
  using ChunkType = chunk::DataChunk::Type;

 public:
  explicit InstantQuerier(DataStorage& storage) : storage_(storage) {}

  template <typename LsIDStorage, typename SampleStorage>
  void query(SampleStorage& samples, const LsIDStorage& label_set_ids, const Timestamp& timestamp) noexcept {
    assert(std::size(samples) == std::size(label_set_ids));

    for (auto&& [sample, ls_id] : std::ranges::views::zip(samples, label_set_ids)) {
      query_sample(sample, ls_id, timestamp);
    }
  }

  template <typename LsIDStorage, typename SampleStorage>
  void query_reload(SampleStorage& samples, const LsIDStorage& label_set_ids, const Timestamp& timestamp) noexcept {
    assert(std::size(samples) == std::size(label_set_ids));

    for (auto&& [sample, ls_id] : std::ranges::views::zip(samples, label_set_ids)) {
      if (series_to_load_.is_set(ls_id)) {
        query_sample(sample, ls_id, timestamp);
      }
    }
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const DataStorage& get_storage() const noexcept { return storage_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE DataStorage& get_storage() noexcept { return storage_; }
  [[nodiscard]] bool need_loading() const noexcept { return series_to_load_.empty() == false; }
  [[nodiscard]] const BareBones::Bitset& get_series_to_load() const noexcept { return series_to_load_; }

 private:
  DataStorage& storage_;
  BareBones::Bitset series_to_load_;

  PROMPP_ALWAYS_INLINE void query_sample(Sample& sample, LabelSetID ls_id, const Timestamp& timestamp) noexcept {
    if (storage_.open_chunks.size() <= ls_id || storage_.open_chunks[ls_id].is_empty()) [[unlikely]] {
      return;
    }
    if (const auto series_last_ts = Decoder::get_series_max_timestamp(storage_, ls_id); timestamp >= series_last_ts) {
      sample = {.timestamp = series_last_ts, .value = Decoder::get_open_chunk_last_value(storage_, storage_.open_chunks[ls_id])};
    } else if (const auto series_first_ts = Decoder::get_series_min_timestamp(storage_, ls_id); timestamp >= series_first_ts) {
      storage_.queried_series_bitmap.set_atomic(ls_id);
      if (storage_.unloaded_series_bitmap.is_set(ls_id)) {
        series_to_load_.set(ls_id);
        return;
      }
      check_inside_series(sample, ls_id, timestamp);
    }
  }

  void check_inside_series(Sample& sample, LabelSetID ls_id, const Timestamp& timestamp) noexcept {
    for (const auto& chunk_data : DataStorage::SeriesChunks(&storage_, ls_id)) {
      if (Decoder::get_chunk_time_interval(chunk_data).contains(timestamp)) {
        Decoder::create_decode_iterator(chunk_data, [&](auto&& begin, auto&& end) PROMPP_LAMBDA_INLINE {
          for (auto sample_it = begin; sample_it != end && sample_it->timestamp <= timestamp; ++sample_it) {
            sample = *sample_it;
          }
        });
      }
    }
  }
};
}  // namespace series_data

static_assert(series_data::LoadableQuerierInterface<series_data::InstantQuerier>);
