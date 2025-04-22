#pragma once

#include "outdated_chunk_merger.h"

namespace series_data {

template <uint8_t kSamplesPerChunk = kSamplesPerChunkDefault>
class OutdatedSampleEncoder {
 public:
  template <EncoderInterface Encoder>
  static void encode(Encoder& encoder, uint32_t ls_id, int64_t timestamp, double value) {
    auto& storage = encoder.storage();
    ++storage.outdated_samples_count;

    if (auto it = storage.outdated_chunks.try_emplace(ls_id, timestamp, value); !it.second) {
      if (it.first->second.encode(timestamp, value) >= kSamplesPerChunk) {
        OutdatedChunkMerger<Encoder> merger{encoder};
        merger.merge(ls_id, it.first->second);
        storage.outdated_chunks.erase(it.first);
      }
    } else {
      ++storage.outdated_chunks_count;
    }
  }

  template <EncoderInterface Encoder>
  static void merge_outdated_chunks(Encoder& encoder) {
    if (encoder.storage().outdated_chunks.empty()) {
      return;
    }
    OutdatedChunkMerger<Encoder> merger{encoder};
    for (const auto& [ls_id, outdated_chunk] : encoder.storage().outdated_chunks) {
      merger.merge(ls_id, outdated_chunk);
    }
    encoder.storage().outdated_chunks.clear();
  }
};

}  // namespace series_data