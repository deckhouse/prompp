#include <benchmark/benchmark.h>

#include "benchmark/statistic.h"
#include "primitives/snug_composites.h"
#include "profiling/profiling.h"
#include "series_index/querier/querier.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using PromPP::Prometheus::LabelMatchers;

using QueryableEncodingBimap = series_index::QueryableEncodingBimap<BareBones::Vector>;
using Querier = series_index::querier::Querier<BareBones::Vector>;

std::string get_lss_file() {
  if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
    return context->operator[]("lss_file");
  }

  return {};
}

const QueryableEncodingBimap& get_lss() {
  static QueryableEncodingBimap lss;
  if (lss.series_count() == 0) {
    std::ifstream infile(get_lss_file(), std::ios_base::binary);
    infile >> lss;
  }

  return lss;
}

const std::array kBenchmarkCases{
    LabelMatchers{
        {.name = "__name__", .value = "container_cpu_usage_seconds_total", .type = PromPP::Prometheus::MatcherType::kExactMatch},
        {.name = "node", .value = ".*", .type = PromPP::Prometheus::MatcherType::kRegexpMatch},
        {.name = "container", .value = "POD", .type = PromPP::Prometheus::MatcherType::kExactNotMatch},
        {.name = "pod", .value = ".*", .type = PromPP::Prometheus::MatcherType::kRegexpMatch},
    },
    LabelMatchers{
        {.name = "__name__", .value = "container_cpu_usage_seconds_total", .type = PromPP::Prometheus::MatcherType::kExactMatch},
        {.name = "node", .value = ".*", .type = PromPP::Prometheus::MatcherType::kRegexpMatch},
        {.name = "container", .value = "POD", .type = PromPP::Prometheus::MatcherType::kExactNotMatch},
        {.name = "pod", .value = ".*", .type = PromPP::Prometheus::MatcherType::kRegexpMatch},
        {.name = "namespace", .value = "d8*", .type = PromPP::Prometheus::MatcherType::kRegexpMatch},
    },
    LabelMatchers{
        {.name = "__name__", .value = "container_cpu_usage_seconds_total", .type = PromPP::Prometheus::MatcherType::kExactMatch},
    },
};

void LssQuery(benchmark::State& state) {
  ZoneScoped;
  const auto& lss = get_lss();

  for ([[maybe_unused]] auto _ : state) {
    auto result = Querier::query(lss, kBenchmarkCases[state.range(0)]);
    benchmark::DoNotOptimize(result);
    benchmark::ClobberMemory();
  }
}

BENCHMARK(LssQuery)->DenseRange(0, kBenchmarkCases.size() - 1, 1)->ComputeStatistics("min", benchmark::min_time);

}  // namespace
