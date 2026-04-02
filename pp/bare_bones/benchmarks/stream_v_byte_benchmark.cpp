#include <benchmark/benchmark.h>

#include <numeric>
#include <ranges>

#include "bare_bones/stream_v_byte.h"
#include "profiling/profiling.h"

namespace {

uint32_t values_count() {
  if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
    const auto& values_str = context->operator[]("values");
    return std::strtoul(values_str.data(), nullptr, 10);
  }

  return {};
}

using Sequence = BareBones::StreamVByte::Sequence<BareBones::StreamVByte::Codec0124, 8>;
using CompactSequence = BareBones::StreamVByte::CompactSequence<BareBones::StreamVByte::Codec0124, BareBones::MemoryWithItemCount, 8>;

template <class Sequence>
void BenchmarkSequencePushBack(benchmark::State& state) {
  ZoneScoped;
  const auto kValuesCount = values_count();

  for ([[maybe_unused]] auto _ : state) {
    Sequence sequence;
    for (const auto value : std::views::iota(0U, kValuesCount)) {
      sequence.push_back(value);
    }
  }

  state.counters["Memory"] = [kValuesCount] {
    Sequence sequence;
    for (const auto value : std::views::iota(0U, kValuesCount)) {
      sequence.push_back(value);
    }
    return sequence.allocated_memory();
  }();
}

template <class Sequence>
void BenchmarkSequenceDecode(benchmark::State& state) {
  ZoneScoped;
  const auto kValuesCount = values_count();

  Sequence sequence;
  for (const auto value : std::views::iota(0U, kValuesCount)) {
    sequence.push_back(value);
  }

  for ([[maybe_unused]] auto _ : state) {
    std::ranges::for_each(sequence, [](auto value) { benchmark::DoNotOptimize(value); });
  }
}

double min_value(const std::vector<double>& v) noexcept {
  return *std::ranges::min_element(v);
}

BENCHMARK(BenchmarkSequencePushBack<Sequence>)->ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkSequencePushBack<CompactSequence>)->ComputeStatistics("min", min_value);

BENCHMARK(BenchmarkSequenceDecode<Sequence>)->ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkSequenceDecode<CompactSequence>)->ComputeStatistics("min", min_value);

}  // namespace
