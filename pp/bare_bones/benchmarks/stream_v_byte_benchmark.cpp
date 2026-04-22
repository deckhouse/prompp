#include <benchmark/benchmark.h>

#include <numeric>
#include <ranges>

#include "bare_bones/stream_v_byte.h"
#include "benchmark/statistic.h"
#include "profiling/profiling.h"

namespace {

constexpr uint32_t kDefaultValuesCount = 1000;

uint32_t values_count() {
  if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
    const auto& values_str = context->operator[]("values");
    if (!values_str.empty()) {
      return std::strtoul(values_str.data(), nullptr, 10);
    }
  }

  return kDefaultValuesCount;
}

using Sequence = BareBones::StreamVByte::Sequence<BareBones::StreamVByte::Codec0124, 8>;
using CompactSequence = BareBones::StreamVByte::CompactSequence<BareBones::StreamVByte::Codec0124, BareBones::MemoryWithItemCount, 8>;

template <class Sequence>
void SequencePushBack(benchmark::State& state) {
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
void SequenceDecode(benchmark::State& state) {
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

BENCHMARK(SequencePushBack<Sequence>)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK(SequencePushBack<CompactSequence>)->ComputeStatistics("min", benchmark::min_time);

BENCHMARK(SequenceDecode<Sequence>)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK(SequenceDecode<CompactSequence>)->ComputeStatistics("min", benchmark::min_time);

}  // namespace
