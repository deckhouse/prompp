#include <benchmark/benchmark.h>

#include <numeric>
#include <random>
#include <ranges>

#include "bare_bones/stream_v_byte.h"
#include "bare_bones/encoding.h"

namespace {
using DataSequence = BareBones::StreamVByte::CompactSequence<BareBones::StreamVByte::Codec1234>;
using EncodedDeltaDataSequence = BareBones::EncodedSequence<BareBones::Encoding::Delta<DataSequence>>;

void BenchmarkDeltaEncoding(benchmark::State& state) {
  const size_t kValuesCount = 1'000'000;
  std::vector<uint32_t> numbers(kValuesCount);
  std::iota(numbers.begin(), numbers.end(), 0);

  const size_t sample_size = kValuesCount / 10;

  std::mt19937 gen(42);

  std::ranges::shuffle(numbers, gen);
  std::vector<uint32_t> sample(numbers.begin(), numbers.begin() + sample_size);
  std::ranges::sort(sample);

  for ([[maybe_unused]] auto _ : state) {
    EncodedDeltaDataSequence sequence;
    for (const auto value : sample) {
      sequence.push_back(value);
    }
    benchmark::DoNotOptimize(sequence);
  }

  EncodedDeltaDataSequence sequence;
  for (const auto value : sample) {
    sequence.push_back(value);
  }

  state.counters["Uncompressed"] =
      benchmark::Counter(static_cast<double>(sample.size() * sizeof(uint32_t)), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);

  state.counters["Compressed"] = benchmark::Counter(static_cast<double>(sequence.allocated_memory()), benchmark::Counter::kDefaults,
                                                    benchmark::Counter::OneK::kIs1024);
}

double min_value(const std::vector<double>& v) noexcept {
  return *std::ranges::min_element(v);
}

BENCHMARK(BenchmarkDeltaEncoding)->Unit(benchmark::kMillisecond)->ComputeStatistics("min", min_value);
} // namespace
