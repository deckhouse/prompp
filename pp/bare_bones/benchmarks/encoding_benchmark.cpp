#include <benchmark/benchmark.h>

#include <fstream>

#include "bare_bones/encoding.h"
#include "benchmark/statistic.h"
#include "profiling/profiling.h"

namespace {

using BareBones::Encoding::DeltaRLE;
using BareBones::StreamVByte::Codec0124;
using BareBones::StreamVByte::Sequence;

template <class Codec, size_t kPreAllocationElementsCount>
using CompactSequence = BareBones::StreamVByte::CompactSequence<Codec, BareBones::MemoryWithItemCount, kPreAllocationElementsCount>;

const BareBones::Vector<uint32_t>& values() {
  static BareBones::Vector<uint32_t> values;
  if (values.empty()) {
    if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
      std::ifstream infile(context->operator[]("encoded_sequence_values_file").data(), std::ios_base::binary);
      infile >> values;
    }
  }

  return values;
}

using EncodedSequence = BareBones::EncodedSequence<DeltaRLE<Sequence<Codec0124, 8>>>;
using EncodedCompactSequence = BareBones::EncodedSequence<DeltaRLE<CompactSequence<Codec0124, 8>>>;

template <class EncodingSequence>
void EncodingSequencePushBack(benchmark::State& state) {
  ZoneScoped;
  const auto& kValues = values();

  for ([[maybe_unused]] auto _ : state) {
    EncodingSequence sequence;
    for (const auto value : kValues) {
      sequence.push_back(value);
    }
  }

  state.counters["Memory"] = [&kValues] {
    EncodingSequence sequence;
    for (const auto value : kValues) {
      sequence.push_back(value);
    }
    return sequence.allocated_memory();
  }();
}

template <class EncodingSequence>
void EncodingSequenceDecode(benchmark::State& state) {
  ZoneScoped;
  const auto& kValues = values();

  EncodingSequence sequence;
  for (const auto value : kValues) {
    sequence.push_back(value);
  }

  for ([[maybe_unused]] auto _ : state) {
    std::ranges::for_each(sequence, [](auto value) { benchmark::DoNotOptimize(value); });
  }
}

BENCHMARK(EncodingSequencePushBack<EncodedSequence>)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK(EncodingSequencePushBack<EncodedCompactSequence>)->ComputeStatistics("min", benchmark::min_time);

BENCHMARK(EncodingSequenceDecode<EncodedSequence>)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK(EncodingSequenceDecode<EncodedCompactSequence>)->ComputeStatistics("min", benchmark::min_time);

}  // namespace
