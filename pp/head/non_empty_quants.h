#pragma once

#include <algorithm>

#include "bare_bones/preprocess.h"
#include "bare_bones/vector.h"
#include "series_data/decoder.h"
#include "series_data/encoder/timestamp/encoder.h"

namespace head {

// Returns the sorted start timestamps of the quant_duration-aligned quants that hold at least one sample.
// Only timestamp streams are read, so the result is valid for unloaded series too.
template <class DataStorage>
[[nodiscard]] BareBones::Vector<int64_t> get_non_empty_quants(const DataStorage& storage, int64_t quant_duration) {
  const auto quant_start = [quant_duration](int64_t timestamp) PROMPP_LAMBDA_INLINE {
    const auto quant = timestamp / quant_duration - (timestamp % quant_duration < 0 ? 1 : 0);
    return quant * quant_duration;
  };

  BareBones::Vector<int64_t> quants;
  const auto add_quant = [&quants](int64_t quant) PROMPP_LAMBDA_INLINE {
    if (quants.empty() || quants.back() != quant) {
      quants.emplace_back(quant);
    }
  };

  for (const auto& chunk_data : storage.chunks()) {
    if (chunk_data.chunk().is_empty()) {
      continue;
    }

    // The interval max of a finalized chunk is the next chunk start, an upper bound of its last sample.
    const auto interval = series_data::Decoder::get_chunk_time_interval(chunk_data);
    if (const auto first_quant = quant_start(interval.min); first_quant == quant_start(interval.max)) [[likely]] {
      add_quant(first_quant);
      continue;
    }

    const auto& timestamp_stream = storage.get_timestamp_stream(chunk_data.chunk().timestamp_id);
    series_data::encoder::timestamp::TimestampDecoder decoder(timestamp_stream.reader());
    for (auto i = timestamp_stream.count(); i != 0; --i) {
      add_quant(quant_start(decoder.decode()));
    }
  }

  // Outdated samples of unloaded series are merged into their chunks only when the series is loaded for recoding.
  for (const auto& [ls_id, chunk] : storage.outdated_chunks) {
    std::ranges::for_each(series_data::decoder::OutdatedDecodeIterator(chunk.samples_count(), chunk.stream().reader()),
                          series_data::decoder::DecodeIteratorSentinel{},
                          [&](const series_data::encoder::Sample& sample) PROMPP_LAMBDA_INLINE { add_quant(quant_start(sample.timestamp)); });
  }

  std::ranges::sort(quants);
  quants.erase(std::ranges::unique(quants).begin(), quants.end());
  return quants;
}

}  // namespace head
