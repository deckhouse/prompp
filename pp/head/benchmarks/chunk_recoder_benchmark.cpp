#include <benchmark/benchmark.h>

#include "benchmark/compact_sample.h"
#include "benchmark/statistic.h"
#include "head/chunk_recoder.h"
#include "profiling/profiling.h"
#include "series_data/decoder.h"
#include "series_data/encoder.h"

namespace {

using DataStorage = series_data::DataStorage;
using PromPP::Primitives::TimeInterval;

const DataStorage& get_data_storage_for_benchmark() {
  static series_data::DataStorage storage;
  if (storage.open_chunks.empty()) [[unlikely]] {
    const auto& samples = benchmark::get_compact_samples();
    series_data::Encoder encoder{storage};
    for (const auto& sample : samples) {
      encoder.encode(sample.series_id(), sample.timestamp(), sample.value());
    }
  }

  return storage;
}

void ChunkRecoder(benchmark::State& state) {
  ZoneScoped;
  const auto& storage = get_data_storage_for_benchmark();

  const auto time_interval = series_data::Decoder::get_time_interval(storage);
  const std::ranges::iota_view<uint32_t, uint32_t> ls_id_set(0, storage.open_chunks.size());
  head::ChunkRecoder recoder(head::ChunkRecoderIterator{ls_id_set.begin(), ls_id_set.end(), storage.open_chunks.size(), &storage, time_interval},
                             time_interval);

  struct {
    TimeInterval interval;
    uint32_t series_id{};
    uint8_t samples_count{};
  } chunk_data;

  for ([[maybe_unused]] auto _ : state) {
    while (recoder.has_more_data()) {
      recoder.recode_next_chunk(chunk_data);
    }
  }
}

BENCHMARK(ChunkRecoder)->ComputeStatistics("min", benchmark::min_time);

}  // namespace
