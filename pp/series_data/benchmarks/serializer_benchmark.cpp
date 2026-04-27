#include <chrono>
#include <fstream>

#include <benchmark/benchmark.h>

#include <iostream>
#include <random>

#include "benchmark/compact_sample.h"
#include "benchmark/statistic.h"
#include "profiling/profiling.h"
#include "series_data/encoder.h"
#include "series_data/querier/query.h"
#include "series_data/serialization/serialized_data.h"

namespace {

using BareBones::StreamVByte::CompactSequence;
using BareBones::StreamVByte::Sequence;
using series_data::serialization::DataSerializer;
using series_data::serialization::SerializedData;
using series_data::serialization::SerializedDataView;

series_data::querier::QueriedChunkList generate_query(uint32_t size) {
  series_data::querier::QueriedChunkList chunk_list;

  std::vector<uint32_t> v(size);
  std::iota(v.begin(), v.end(), 0);

  std::mt19937 g(42);
  std::ranges::shuffle(v, g);
  v.resize(v.size() / 10);

  chunk_list.reserve(v.size());
  for (uint32_t ls_id : v) {
    chunk_list.emplace_back(ls_id);
  }

  return chunk_list;
}

void WalSerializedData(benchmark::State& state) {
  ZoneScoped;
  const auto& samples = benchmark::get_compact_samples();
  const double percent = static_cast<double>(state.range(0)) / 100.0;
  const auto [min, max] = std::ranges::minmax_element(samples, [](auto a, auto b) { return a.timestamp() < b.timestamp(); });
  const auto min_ts = min->timestamp();
  const auto max_ts = max->timestamp();
  const auto delta_ts = max_ts - min_ts;

  series_data::DataStorage storage;
  series_data::Encoder encoder{storage};

  for (const auto& sample : samples) {
    if (sample.timestamp() <= min_ts + static_cast<int64_t>(static_cast<double>(delta_ts) * percent)) {
      encoder.encode(sample.series_id(), sample.timestamp(), sample.value());
    }
  }

  const series_data::querier::QueriedChunkList chunk_list = generate_query(storage.open_chunks.size());

  for ([[maybe_unused]] auto _ : state) {
    SerializedData serialized = DataSerializer{storage}.serialize(chunk_list);
    benchmark::DoNotOptimize(serialized);
  }

  {
    const SerializedData serialized = DataSerializer{storage}.serialize(chunk_list);
    state.counters["Total Size"] = benchmark::Counter(serialized.allocated_memory(), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);
  }
}

void WalConstantSerializedData(benchmark::State& state) {
  ZoneScoped;
  const auto& samples = benchmark::get_compact_samples();
  const double percent = static_cast<double>(state.range(0)) / 100.0;
  const auto [min, max] = std::ranges::minmax_element(samples, [](auto a, auto b) { return a.timestamp() < b.timestamp(); });
  const auto min_ts = min->timestamp();
  const auto max_ts = max->timestamp();
  const auto delta_ts = max_ts - min_ts;

  series_data::DataStorage storage;
  series_data::Encoder encoder{storage};

  for (const auto& sample : samples) {
    if (sample.timestamp() <= min_ts + static_cast<int64_t>(static_cast<double>(delta_ts) * percent)) {
      encoder.encode(sample.series_id(), sample.timestamp(), sample.series_id());
    }
  }

  const series_data::querier::QueriedChunkList chunk_list = generate_query(storage.open_chunks.size());

  for ([[maybe_unused]] auto _ : state) {
    SerializedData serialized = DataSerializer{storage}.serialize(chunk_list);
    benchmark::DoNotOptimize(serialized);
  }

  {
    const SerializedData serialized = DataSerializer{storage}.serialize(chunk_list);
    state.counters["Total Size"] = benchmark::Counter(serialized.allocated_memory(), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);
  }
}

BENCHMARK(WalSerializedData)->Arg(25)->Arg(50)->Arg(75)->Arg(100)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK(WalConstantSerializedData)->Arg(25)->Arg(50)->Arg(75)->Arg(100)->ComputeStatistics("min", benchmark::min_time);

}  // namespace
