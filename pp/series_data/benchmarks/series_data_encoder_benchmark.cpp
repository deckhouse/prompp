#include <chrono>
#include <fstream>

#include <benchmark/benchmark.h>

#include "profiling/profiling.h"

#include "bare_bones/preprocess.h"
#include "primitives/sample.h"
#include "series_data/encoder.h"

namespace {
// timestamp min value
constexpr PromPP::Primitives::Sample::timestamp_type ts_min = 1698395400012;

struct PROMPP_ATTRIBUTE_PACKED sample_with_lsid {
  PromPP::Primitives::Sample::value_type sample_value;
  uint32_t sample_ts;
  uint32_t labelset_id;
};

const BareBones::Vector<sample_with_lsid>& get_samples_for_benchmark() {
  constexpr auto get_file_name = [] -> std::string {
    if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
      return context->operator[]("sde_file");
    }

    return {};
  };

  static BareBones::Vector<sample_with_lsid> samples_from_file;
  if (samples_from_file.empty()) [[unlikely]] {
    std::ifstream istrm(get_file_name(), std::ios::binary);
    istrm >> samples_from_file;
  }

  return samples_from_file;
}

void BenchmarkSeriesDataEncoder(benchmark::State& state) {
  ZoneScoped;
  const auto& samples = get_samples_for_benchmark();

  series_data::DataStorage storage;
  series_data::Encoder encoder{storage};

  for ([[maybe_unused]] auto _ : state) {
    for (const auto& sample : samples) {
      encoder.encode(sample.labelset_id, ts_min + static_cast<PromPP::Primitives::Sample::timestamp_type>(sample.sample_ts), sample.sample_value);
    }
  }

  state.counters["Items"] = benchmark::Counter(samples.size());
  state.counters["Time/item"] = benchmark::Counter(samples.size(), benchmark::Counter::kIsRate | benchmark::Counter::kInvert);

  state.counters["Memory"] =
      benchmark::Counter(static_cast<double>(storage.allocated_memory()), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);
}

BENCHMARK(BenchmarkSeriesDataEncoder)->ComputeStatistics("min", [](const std::vector<double>& v) { return *std::ranges::min_element(v); });

}  // namespace
