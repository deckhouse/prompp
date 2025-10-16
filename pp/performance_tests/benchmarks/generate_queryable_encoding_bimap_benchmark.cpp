// ./bazel-bin/performance_tests/benchmarks/generate_lss --benchmark_context=wal_file="performance_tests/test_data/new/lss_real"
// --benchmark_counters_tabular=true
// --benchmark_repetitions=25
#include <benchmark/benchmark.h>

#include <chrono>
#include <fstream>
#include <string>

#include "primitives/snug_composites.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {
using Lss =
    series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, BareBones::Vector, series_index::trie::CedarTrie>;

std::string GetWalFileName() {
  if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
    return context->operator[]("wal_file");
  }
  return {};
}

std::shared_ptr<Lss> LoadLssFromFile() {
  auto file_name = GetWalFileName();
  auto lss = std::make_shared<Lss>();

  std::ifstream istrm(file_name, std::ios::binary);
  istrm >> *lss;
  return lss;
}

void BM_EmplaceAllLabelSets(benchmark::State& state) {
  using std::chrono::nanoseconds;
  using std::chrono::steady_clock;

  static auto lss = LoadLssFromFile();

  for ([[maybe_unused]] auto _ : state) {
    Lss lss_copy;

    for (auto label_set : *lss) {
      lss_copy.find_or_emplace(label_set);
    }

    benchmark::DoNotOptimize(lss_copy);

    state.counters["Items"] = benchmark::Counter(lss_copy.size());
    state.counters["Time/item"] = benchmark::Counter(lss_copy.size(), benchmark::Counter::kIsRate | benchmark::Counter::kInvert);
    state.counters["Memory"] =
        benchmark::Counter(static_cast<double>(lss_copy.allocated_memory()), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);
  }
}

void BM_FindOnlyEmplaceAllLabelSets(benchmark::State& state) {
  using std::chrono::nanoseconds;
  using std::chrono::steady_clock;

  static auto lss = LoadLssFromFile();
  Lss lss_copy;

  for (auto label_set : *lss) {
    lss_copy.find_or_emplace(label_set);
  }

  for ([[maybe_unused]] auto _ : state) {
    for (auto label_set : *lss) {
      lss_copy.find_or_emplace(label_set);
    }

    state.counters["Items"] = benchmark::Counter(lss_copy.size());
    state.counters["Time/item"] = benchmark::Counter(lss_copy.size(), benchmark::Counter::kIsRate | benchmark::Counter::kInvert);
    state.counters["Memory"] =
        benchmark::Counter(static_cast<double>(lss_copy.allocated_memory()), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);
  }
  benchmark::DoNotOptimize(lss_copy);
}

void BM_ReadAllLabelSets(benchmark::State& state) {
  using std::chrono::nanoseconds;
  using std::chrono::steady_clock;

  static auto lss = LoadLssFromFile();
  Lss lss_copy;

  for (auto label_set : *lss) {
    lss_copy.find_or_emplace(label_set);
  }

  size_t hash = 0;

  for ([[maybe_unused]] auto _ : state) {
    for (auto label_set : lss_copy) {
      hash ^= hash_value(label_set);
    }

    state.counters["Items"] = benchmark::Counter(lss_copy.size());
    state.counters["Time/item"] = benchmark::Counter(lss_copy.size(), benchmark::Counter::kIsRate | benchmark::Counter::kInvert);
    state.counters["Memory"] =
        benchmark::Counter(static_cast<double>(lss_copy.allocated_memory()), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);
  }
  benchmark::DoNotOptimize(hash);
}

BENCHMARK(BM_EmplaceAllLabelSets)->ComputeStatistics("min", [](const std::vector<double>& v) { return *std::ranges::min_element(v); });
BENCHMARK(BM_FindOnlyEmplaceAllLabelSets)->ComputeStatistics("min", [](const std::vector<double>& v) { return *std::ranges::min_element(v); });
BENCHMARK(BM_ReadAllLabelSets)->ComputeStatistics("min", [](const std::vector<double>& v) { return *std::ranges::min_element(v); });

}  // namespace
