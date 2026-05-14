#include <benchmark/benchmark.h>

#include "benchmark/statistic.h"
#include "primitives/snug_composites.h"
#include "profiling/profiling.h"

namespace {

using QueryableEncodingBimap = PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<BareBones::Vector>;

std::string get_lss_file() {
  if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
    return context->operator[]("lss_file");
  }

  return {};
}

QueryableEncodingBimap& get_lss() {
  static QueryableEncodingBimap lss;
  if (lss.items_count() == 0) {
    std::ifstream infile(get_lss_file(), std::ios_base::binary);
    infile >> lss;
  }

  return lss;
}

void FindOrEmplaceWithEmplace(benchmark::State& state) {
  ZoneScoped;
  const auto& lss = get_lss();

  for ([[maybe_unused]] auto _ : state) {
    QueryableEncodingBimap lss2;
    for (const auto& label_set : lss) {
      lss2.find_or_emplace(label_set);
    }
  }
}

void FindOrEmplaceWithFind(benchmark::State& state) {
  ZoneScoped;
  auto& lss = get_lss();

  for ([[maybe_unused]] auto _ : state) {
    for (const auto& label_set : lss) {
      lss.find_or_emplace(label_set);
    }
  }
}

BENCHMARK(FindOrEmplaceWithEmplace)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK(FindOrEmplaceWithFind)->ComputeStatistics("min", benchmark::min_time);

}  // namespace
