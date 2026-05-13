#include <fstream>

#include <benchmark/benchmark.h>

#include "benchmark/statistic.h"
#include "primitives/timeseries.h"
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Warray-bounds"
#include "profiling/profiling.h"
#include "wal/hashdex/scraper/scraper.h"

namespace {

using PromPP::WAL::hashdex::scraper::PrometheusParser;
using PromPP::WAL::hashdex::scraper::PrometheusScraper;

std::string get_file_content() {
  if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
    std::ifstream t(context->operator[]("prom_scraper_file"));
    return std::string((std::istreambuf_iterator(t)), std::istreambuf_iterator<char>());
  }

  return {};
};

void Parser(benchmark::State& state) {
  ZoneScoped;
  const auto str = get_file_content();

  PrometheusParser parser;

  for ([[maybe_unused]] auto _ : state) {
    parser.tokenizer().tokenize(str);
    while (parser.tokenizer().next() != PromPP::Prometheus::textparse::Token::kEOF) {
      benchmark::DoNotOptimize(parser);
    }
  }
}

void ScraperParse(benchmark::State& state) {
  ZoneScoped;
  const auto str = get_file_content();

  std::string tmp_str;
  tmp_str.resize(str.size());

  for ([[maybe_unused]] auto _ : state) {
    std::memcpy(tmp_str.data(), str.data(), str.size());
    PrometheusScraper scraper;
    std::ignore = scraper.parse(tmp_str, 0);
  }

  {
    PrometheusScraper scraper;
    auto tmp_str2 = str;
    std::ignore = scraper.parse(tmp_str2, 0);
    state.counters["Alloc"] =
        benchmark::Counter(static_cast<double>(scraper.allocated_memory()), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);
  }
}

void ScraperRead(benchmark::State& state) {
  ZoneScoped;
  auto str = get_file_content();

  PrometheusScraper scraper;
  std::ignore = scraper.parse(str, 0);

  PromPP::Primitives::TimeseriesSemiview ts_buf;

  for ([[maybe_unused]] auto _ : state) {
    for (auto& metric : scraper.metrics()) {
      if (metric.hash() % 2 == 0) {
        ts_buf.clear();
        metric.read(ts_buf);
      }
    }
  }
}

BENCHMARK(Parser)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK(ScraperParse)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK(ScraperRead)->ComputeStatistics("min", benchmark::min_time);

}  // namespace
