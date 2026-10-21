#include <benchmark/benchmark.h>

#include <algorithm>
#include <cstdint>
#include <cstdlib>
#include <fstream>
#include <stdexcept>
#include <string>

#include "benchmark/statistic.h"
#include "profiling/profiling.h"
#include "series_index/querier/querier.h"
#include "series_index/queryable_encoding_bimap.h"

namespace {

using PromPP::Prometheus::LabelMatchers;
using PromPP::Prometheus::MatcherType;

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
    if (!infile) {
      throw std::runtime_error("Missing label-set benchmark fixture");
    }
    infile >> lss;
    if (lss.items_count() == 0) {
      throw std::runtime_error("Empty label-set benchmark fixture");
    }
  }

  return lss;
}

struct BenchmarkCase {
  LabelMatchers matchers;
};

// Single exact matcher: the smallest set is decoded and returned as is (short RLE runs).
const BenchmarkCase kMetricOnly{
    .matchers =
        {
            {.name = "__name__", .value = "container_cpu_usage_seconds_total", .type = MatcherType::kExactMatch},
        },
};

// Two postings made of long RLE runs of consecutive ids.
const BenchmarkCase kMetricByTarget{
    .matchers =
        {
            {.name = "__name__", .value = "apiserver_request_duration_seconds_bucket", .type = MatcherType::kExactMatch},
            {.name = "instance", .value = "192.168.199.131:6443", .type = MatcherType::kExactMatch},
        },
};

// Filter of similar size whose ids interleave with the smallest set (short runs).
const BenchmarkCase kMetricByInterleavedLabel{
    .matchers =
        {
            {.name = "__name__", .value = "apiserver_request_duration_seconds_bucket", .type = MatcherType::kExactMatch},
            {.name = "scope", .value = "resource", .type = MatcherType::kExactMatch},
        },
};

// Tiny smallest set against a filter hundreds of times larger.
const BenchmarkCase kSmallMetricByLargeLabel{
    .matchers =
        {
            {.name = "__name__", .value = "up", .type = MatcherType::kExactMatch},
            {.name = "job", .value = "kube-state-metrics", .type = MatcherType::kExactMatch},
        },
};

// `l!=""` becomes a positive kAllMatch over every series having the label.
const BenchmarkCase kMetricWithLabelPresent{
    .matchers =
        {
            {.name = "__name__", .value = "container_cpu_usage_seconds_total", .type = MatcherType::kExactMatch},
            {.name = "container", .value = "", .type = MatcherType::kExactNotMatch},
        },
};

// `l=""` becomes a negative kAllMatch that removes most of the smallest set.
const BenchmarkCase kMetricWithLabelAbsent{
    .matchers =
        {
            {.name = "__name__", .value = "apiserver_request_duration_seconds_bucket", .type = MatcherType::kExactMatch},
            {.name = "resource", .value = "", .type = MatcherType::kExactMatch},
        },
};

// Positive filter with two values.
const BenchmarkCase kMetricByTwoValues{
    .matchers =
        {
            {.name = "__name__", .value = "apiserver_request_duration_seconds_bucket", .type = MatcherType::kExactMatch},
            {.name = "verb", .value = "GET|LIST", .type = MatcherType::kRegexpMatch},
        },
};

// Positive filter with dozens of values, much larger than the smallest set.
const BenchmarkCase kMetricByManyValues{
    .matchers =
        {
            {.name = "__name__", .value = "container_cpu_usage_seconds_total", .type = MatcherType::kExactMatch},
            {.name = "namespace", .value = "d8-.*", .type = MatcherType::kRegexpMatch},
        },
};

// Negative filter with one value.
const BenchmarkCase kMetricExcludingValue{
    .matchers =
        {
            {.name = "__name__", .value = "apiserver_request_duration_seconds_bucket", .type = MatcherType::kExactMatch},
            {.name = "verb", .value = "LIST", .type = MatcherType::kExactNotMatch},
        },
};

// Negative filter with two values.
const BenchmarkCase kMetricExcludingValues{
    .matchers =
        {
            {.name = "__name__", .value = "apiserver_request_duration_seconds_bucket", .type = MatcherType::kExactMatch},
            {.name = "verb", .value = "GET|LIST", .type = MatcherType::kRegexpNotMatch},
        },
};

// `l=~"|v"` becomes a negative filter over every other value of the label (hundreds of postings).
const BenchmarkCase kMetricByEmptyOrValue{
    .matchers =
        {
            {.name = "__name__", .value = "apiserver_request_duration_seconds_bucket", .type = MatcherType::kExactMatch},
            {.name = "resource", .value = "|pods", .type = MatcherType::kRegexpMatch},
        },
};

// The smallest set is a union of two postings whose ids interleave (short runs).
const BenchmarkCase kTwoMetricsUnion{
    .matchers =
        {
            {.name = "__name__", .value = "container_cpu_usage_seconds_total|container_memory_failures_total", .type = MatcherType::kRegexpMatch},
        },
};

// The smallest set is a union of a hundred postings made of long runs.
const BenchmarkCase kMetricPrefixUnion{
    .matchers =
        {
            {.name = "__name__", .value = "apiserver_.+", .type = MatcherType::kRegexpMatch},
        },
};

// Typical dashboard selector mixing an exact filter, a label presence check and an exclusion.
const BenchmarkCase kDashboardQuery{
    .matchers =
        {
            {.name = "__name__", .value = "container_cpu_usage_seconds_total", .type = MatcherType::kExactMatch},
            {.name = "namespace", .value = "d8-monitoring", .type = MatcherType::kExactMatch},
            {.name = "container", .value = "", .type = MatcherType::kExactNotMatch},
            {.name = "container", .value = "POD", .type = MatcherType::kExactNotMatch},
        },
};

// The first two sets do not intersect; the remaining filter is the most expensive one.
const BenchmarkCase kEmptyIntersection{
    .matchers =
        {
            {.name = "__name__", .value = "apiserver_request_duration_seconds_bucket", .type = MatcherType::kExactMatch},
            {.name = "node", .value = "kube-node-test-d-magton-e3d1bdf6-74547-hkcrr", .type = MatcherType::kExactMatch},
            {.name = "verb", .value = "GET|LIST", .type = MatcherType::kRegexpMatch},
        },
};

void LssQuery(benchmark::State& state, const BenchmarkCase& benchmark_case) {
  ZoneScoped;
  const auto& lss = get_lss();
  const auto expected = Querier::query(lss, benchmark_case.matchers);
  state.counters["IndexMemory"] = static_cast<double>(lss.allocated_memory());
  state.counters["Matches"] = expected.series_ids.size();
  uint64_t checksum = 0;
  for (auto id : expected.series_ids) {
    checksum += id;
  }
  state.counters["Checksum"] = static_cast<double>(checksum);
  if (const auto prefix = std::getenv("PROMPP_QUERY_ORACLE_PREFIX")) {
    auto name = std::string(state.name());
    std::ranges::replace(name, '/', '_');
    std::ofstream oracle(std::string(prefix) + name, std::ios::binary | std::ios::trunc);
    if (!expected.series_ids.empty()) {
      oracle.write(reinterpret_cast<const char*>(expected.series_ids.begin()), static_cast<std::streamsize>(expected.series_ids.size() * sizeof(uint32_t)));
    }
    if (!oracle) {
      throw std::runtime_error("Cannot write query correctness oracle");
    }
  }

  for ([[maybe_unused]] auto _ : state) {
    auto result = Querier::query(lss, benchmark_case.matchers);
    benchmark::DoNotOptimize(result);
    benchmark::ClobberMemory();
  }
}

BENCHMARK_CAPTURE(LssQuery, MetricOnly, kMetricOnly)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, MetricByTarget, kMetricByTarget)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, MetricByInterleavedLabel, kMetricByInterleavedLabel)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, SmallMetricByLargeLabel, kSmallMetricByLargeLabel)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, MetricWithLabelPresent, kMetricWithLabelPresent)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, MetricWithLabelAbsent, kMetricWithLabelAbsent)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, MetricByTwoValues, kMetricByTwoValues)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, MetricByManyValues, kMetricByManyValues)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, MetricExcludingValue, kMetricExcludingValue)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, MetricExcludingValues, kMetricExcludingValues)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, MetricByEmptyOrValue, kMetricByEmptyOrValue)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, TwoMetricsUnion, kTwoMetricsUnion)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, MetricPrefixUnion, kMetricPrefixUnion)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, DashboardQuery, kDashboardQuery)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK_CAPTURE(LssQuery, EmptyIntersection, kEmptyIntersection)->ComputeStatistics("min", benchmark::min_time);

}  // namespace
