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
  // Distinct quants are few, so a sorted insert costs less than a sort of the duplicates.
  const auto add_quant = [&quants](int64_t quant) PROMPP_LAMBDA_INLINE {
    if (const auto it = std::ranges::lower_bound(quants, quant); it == quants.end() || *it != quant) {
      quants.insert(it, quant);
    }
  };

  // Every timestamp of a live sequence belongs to a chunk, and a sequence shared by many series is read once.
  for (const auto& sequence : storage.timestamp_store.sequences()) {
    const auto first_quant = quant_start(series_data::encoder::timestamp::TimestampDecoder::decode_first(sequence.stream.reader()));
    add_quant(first_quant);
    if (first_quant == quant_start(sequence.last_timestamp)) [[likely]] {
      continue;
    }

    series_data::encoder::timestamp::TimestampDecoder decoder(sequence.stream.reader());
    auto previous_quant = first_quant;
    for (auto i = sequence.stream.count(); i != 0; --i) {
      if (const auto quant = quant_start(decoder.decode()); quant != previous_quant) {
        add_quant(quant);
        previous_quant = quant;
      }
    }
  }

  // Outdated samples are not in the timestamp store until they are merged into chunks.
  for (const auto& [ls_id, chunk] : storage.outdated_chunks) {
    std::ranges::for_each(series_data::decoder::OutdatedDecodeIterator(chunk.samples_count(), chunk.stream().reader()),
                          series_data::decoder::DecodeIteratorSentinel{},
                          [&](const series_data::encoder::Sample& sample) PROMPP_LAMBDA_INLINE { add_quant(quant_start(sample.timestamp)); });
  }

  return quants;
}

}  // namespace head
