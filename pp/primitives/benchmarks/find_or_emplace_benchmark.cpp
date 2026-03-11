#include <benchmark/benchmark.h>

#include "primitives/snug_composites.h"
#include "profiling/profiling.h"

namespace {

using QueryableEncodingBimap = PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<BareBones::Vector>;

std::string_view get_lss_file() {
  if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
    return context->operator[]("lss_file");
  }

  return {};
}

QueryableEncodingBimap& get_lss() {
  static QueryableEncodingBimap lss;
  if (lss.size() == 0) {
    std::ifstream infile(get_lss_file().data(), std::ios_base::binary);
    infile >> lss;
  }

  return lss;
}

void BenchmarkFindOrEmplaceWithEmplace(benchmark::State& state) {
  ZoneScoped;
  const auto& lss = get_lss();

  for ([[maybe_unused]] auto _ : state) {
    QueryableEncodingBimap lss2;
    for (const auto& label_set : lss) {
      lss2.find_or_emplace(label_set);
    }
  }
}

void BenchmarkFindOrEmplaceWithFind(benchmark::State& state) {
  ZoneScoped;
  auto& lss = get_lss();

  for ([[maybe_unused]] auto _ : state) {
    for (const auto& label_set : lss) {
      lss.find_or_emplace(label_set);
    }
  }
}

double min_value(const std::vector<double>& v) noexcept {
  return *std::ranges::min_element(v);
}

BENCHMARK(BenchmarkFindOrEmplaceWithEmplace)->ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkFindOrEmplaceWithFind)->ComputeStatistics("min", min_value);

}  // namespace
