#include <benchmark/benchmark.h>

#include <fstream>

#include "benchmark/statistic.h"
#include "primitives/snug_composites.h"
#include "profiling/profiling.h"
#include "series_index/querier/querier.h"
#include "series_index/queryable_encoding_bimap.h"

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
  if (lss.items_count() == 0) {
    std::ifstream infile(get_lss_file(), std::ios_base::binary);
    infile >> lss;
  }

  return lss;
}

struct BenchmarkCase {
  LabelMatchers matchers;
};

const BenchmarkCase kExactSmallMetric{
    .matchers =
        {
            {.name = "__name__", .value = "container_cpu_usage_seconds_total", .type = PromPP::Prometheus::MatcherType::kExactMatch},
            {.name = "node", .value = "kube-node-test-d-magton-e3d1bdf6-74547-hkcrr", .type = PromPP::Prometheus::MatcherType::kExactMatch},
        },
};

const BenchmarkCase kLargeMetricWithSmallerExactMatcher{
    .matchers =
        {
            {.name = "__name__", .value = "apiserver_request_duration_seconds_bucket", .type = PromPP::Prometheus::MatcherType::kExactMatch},
            {.name = "instance", .value = "192.168.199.131:6443", .type = PromPP::Prometheus::MatcherType::kExactMatch},
        },
};

const BenchmarkCase kLargeMetricWithMidCardinalityExactMatcher{
    .matchers =
        {
            {.name = "__name__", .value = "apiserver_request_duration_seconds_bucket", .type = PromPP::Prometheus::MatcherType::kExactMatch},
            {.name = "scope", .value = "resource", .type = PromPP::Prometheus::MatcherType::kExactMatch},
        },
};

const BenchmarkCase kHeapMergeBeforeIntersection{
    .matchers =
        {
            {.name = "__name__", .value = "apiserver_request_duration_seconds_bucket", .type = PromPP::Prometheus::MatcherType::kExactMatch},
            {.name = "verb", .value = "GET|LIST", .type = PromPP::Prometheus::MatcherType::kRegexpMatch},
        },
};

const BenchmarkCase kTwoPositiveIntersections{
    .matchers =
        {
            {.name = "__name__", .value = "apiserver_request_duration_seconds_bucket", .type = PromPP::Prometheus::MatcherType::kExactMatch},
            {.name = "scope", .value = "resource", .type = PromPP::Prometheus::MatcherType::kExactMatch},
            {.name = "verb", .value = "GET|LIST", .type = PromPP::Prometheus::MatcherType::kRegexpMatch},
        },
};

const BenchmarkCase kOnePostingNegative{
    .matchers =
        {
            {.name = "__name__", .value = "apiserver_request_duration_seconds_bucket", .type = PromPP::Prometheus::MatcherType::kExactMatch},
            {.name = "verb", .value = "LIST", .type = PromPP::Prometheus::MatcherType::kExactNotMatch},
        },
};

const BenchmarkCase kMultiPostingNegative{
    .matchers =
        {
            {.name = "__name__", .value = "apiserver_request_duration_seconds_bucket", .type = PromPP::Prometheus::MatcherType::kExactMatch},
            {.name = "verb", .value = "GET|LIST", .type = PromPP::Prometheus::MatcherType::kRegexpNotMatch},
        },
};

const BenchmarkCase kHeapMergeOnly{
    .matchers = {{.name = "__name__",
                  .value = "container_cpu_usage_seconds_total|container_memory_failures_total",
                  .type = PromPP::Prometheus::MatcherType::kRegexpMatch}},
};

const BenchmarkCase kLargeMetricWithTwoBucketValues{
    .matchers =
        {
            {.name = "__name__", .value = "apiserver_request_duration_seconds_bucket", .type = PromPP::Prometheus::MatcherType::kExactMatch},
            {.name = "le", .value = "0.1|1", .type = PromPP::Prometheus::MatcherType::kRegexpMatch},
        },
};

const BenchmarkCase kLargeMetricUnion{
    .matchers = {{.name = "__name__",
                  .value = "apiserver_request_slo_duration_seconds_bucket|apiserver_request_duration_seconds_bucket",
                  .type = PromPP::Prometheus::MatcherType::kRegexpMatch}},
};

void LssQuery(benchmark::State& state, const BenchmarkCase& benchmark_case) {
  ZoneScoped;
  const auto& lss = get_lss();

  for ([[maybe_unused]] auto _ : state) {
    auto result = Querier::query(lss, benchmark_case.matchers);
    benchmark::DoNotOptimize(result);
    benchmark::ClobberMemory();
  }
}

BENCHMARK_CAPTURE(LssQuery, SmallestPostingDrivesAllocation, kExactSmallMetric)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, LargeMetricWithSmallerExactMatcher, kLargeMetricWithSmallerExactMatcher)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, LargeMetricWithMidCardinalityExactMatcher, kLargeMetricWithMidCardinalityExactMatcher)
    ->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, HeapMergeBeforeIntersection, kHeapMergeBeforeIntersection)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, TwoPositiveIntersections, kTwoPositiveIntersections)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, OnePostingNegative, kOnePostingNegative)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, MultiPostingNegative, kMultiPostingNegative)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, HeapMergeOnly, kHeapMergeOnly)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, LargeMetricWithTwoBucketValues, kLargeMetricWithTwoBucketValues)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, HeapMergeOfLargePostings, kLargeMetricUnion)->ComputeStatistics("min", benchmark::min_time);

}  // namespace
