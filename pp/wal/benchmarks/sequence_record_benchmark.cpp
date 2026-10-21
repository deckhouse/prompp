#include <benchmark/benchmark.h>

#include <algorithm>
#include <cstdint>
#include <fstream>
#include <numeric>
#include <sstream>
#include <stdexcept>
#include <type_traits>

#include "benchmark/statistic.h"
#include "wal/wal.h"

namespace {
const BareBones::Vector<uint32_t>& replay_values() {
  static const auto values = [] {
    BareBones::Vector<uint32_t> result;
    const auto& context = benchmark::internal::GetGlobalContext();
    if (context == nullptr) {
      throw std::runtime_error("Missing benchmark context");
    }
    std::ifstream in(context->at("encoded_sequence_values_file"), std::ios::binary);
    in >> result;
    if (result.empty()) {
      throw std::runtime_error("Missing WAL replay fixture");
    }
    return result;
  }();
  return values;
}

template <uint8_t Id>
void WalSequenceReplay(benchmark::State& state) {
  const auto& values = replay_values();
  const auto expected = std::accumulate(values.begin(), values.end(), uint64_t{});
  using Sequence = std::conditional_t<Id == 1, PromPP::WAL::LabelIdSequence, PromPP::WAL::TimestampSequence>;
  Sequence sequence;
  std::ranges::copy(values, std::back_inserter(sequence));
  sequence.flush();
  std::ostringstream output;
  PromPP::WAL::detail::write_sequence_record<Id>(output, sequence);
  const auto record = output.str();
  uint64_t checksum = 0;
  for ([[maybe_unused]] auto _ : state) {
    std::istringstream input(record);
    PromPP::WAL::SequenceStore store;
    PromPP::WAL::detail::read_sequence_record<Id>(input, store);
    checksum = 0;
    for (const auto value : typename Sequence::View(store.view())) {
      checksum += value;
    }
    benchmark::DoNotOptimize(checksum);
    benchmark::ClobberMemory();
  }
  if (checksum != expected) {
    state.SkipWithError("WAL replay checksum mismatch");
  }
  state.counters["Values"] = values.size();
  state.counters["RecordBytes"] = static_cast<double>(record.size());
}
BENCHMARK_TEMPLATE(WalSequenceReplay, 1)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_TEMPLATE(WalSequenceReplay, 2)->ComputeStatistics("min", benchmark::min_time);
}  // namespace
