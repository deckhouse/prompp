#include <benchmark/benchmark.h>

#include <fstream>

#include "bare_bones/encoding.h"

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
      std::ifstream infile(context->operator[]("values_file").data(), std::ios_base::binary);
      infile >> values;
    }
  }

  return values;
}

template <template <class, size_t> class Sequence>
void EncodingSequencePushBack(benchmark::State& state) {
  const auto& kValues = values();
  using EncodingSequence = BareBones::EncodedSequence<DeltaRLE<Sequence<Codec0124, 8>>>;

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

template <template <class, size_t> class Sequence>
void EncodingSequenceDecode(benchmark::State& state) {
  const auto& kValues = values();
  using EncodingSequence = BareBones::EncodedSequence<DeltaRLE<Sequence<Codec0124, 8>>>;

  EncodingSequence sequence;
  for (const auto value : kValues) {
    sequence.push_back(value);
  }

  for ([[maybe_unused]] auto _ : state) {
    std::ranges::for_each(sequence, [](auto value) { benchmark::DoNotOptimize(value); });
  }
}

double min_value(const std::vector<double>& v) noexcept {
  return *std::ranges::min_element(v);
}

BENCHMARK(EncodingSequencePushBack<Sequence>)->ComputeStatistics("min", min_value);
BENCHMARK(EncodingSequencePushBack<CompactSequence>)->ComputeStatistics("min", min_value);

BENCHMARK(EncodingSequenceDecode<Sequence>)->ComputeStatistics("min", min_value);
BENCHMARK(EncodingSequenceDecode<CompactSequence>)->ComputeStatistics("min", min_value);

}  // namespace
