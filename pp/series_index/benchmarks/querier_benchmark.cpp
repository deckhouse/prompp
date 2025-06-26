#include <benchmark/benchmark.h>

#include "primitives/snug_composites.h"
#include "series_index/querier/querier.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {

using PromPP::Prometheus::LabelMatchers;

using TrieIndex = series_index::TrieIndex<::series_index::trie::CedarTrie, series_index::trie::CedarMatchesList>;
using QueryableEncodingBimap =
    series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, BareBones::Vector, TrieIndex>;
using Querier = series_index::querier::Querier<QueryableEncodingBimap, BareBones::Vector>;

std::string_view get_lss_file() {
  if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
    return context->operator[]("lss_file");
  }

  return {};
}

const QueryableEncodingBimap& get_lss() {
  static QueryableEncodingBimap lss;
  if (lss.size() == 0) {
    std::ifstream infile(get_lss_file().data(), std::ios_base::binary);
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

void BenchmarkQuery(benchmark::State& state) {
  Querier querier(get_lss());

  for ([[maybe_unused]] auto _ : state) {
    auto result = querier.query(kBenchmarkCases[state.range(0)]);
    benchmark::DoNotOptimize(result);
    benchmark::ClobberMemory();
  }
}

double min_value(const std::vector<double>& v) noexcept {
  return *std::ranges::min_element(v);
}

BENCHMARK(BenchmarkQuery)->DenseRange(0, kBenchmarkCases.size() - 1, 1)->ComputeStatistics("min", min_value);

}  // namespace
