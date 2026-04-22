#include <benchmark/benchmark.h>

#include "benchmark/compact_sample.h"
#include "benchmark/statistic.h"
#include "profiling/profiling.h"
#include "series_data/encoder.h"

namespace {

void BenchmarkSeriesDataEncoder(benchmark::State& state) {
  ZoneScoped;
  const auto& samples = benchmark::get_compact_samples();

  series_data::DataStorage storage;
  series_data::Encoder encoder{storage};
  const auto arena_guard = storage.thread_arena_guard();

  for ([[maybe_unused]] auto _ : state) {
    for (const auto& sample : samples) {
      encoder.encode(sample.series_id(), sample.timestamp(), sample.value());
    }
  }

  state.counters["Items"] = benchmark::Counter(samples.size());
  state.counters["Time/item"] = benchmark::Counter(samples.size(), benchmark::Counter::kIsRate | benchmark::Counter::kInvert);

  state.counters["Memory"] =
      benchmark::Counter(static_cast<double>(storage.allocated_memory()), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);
}

BENCHMARK(BenchmarkSeriesDataEncoder)->ComputeStatistics("min", benchmark::min_time);

}  // namespace
