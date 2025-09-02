#include <fstream>

#include <benchmark/benchmark.h>

#include "primitives/timeseries.h"
#include "wal/hashdex/scraper/scraper.h"

namespace {

using PromPP::WAL::hashdex::scraper::PrometheusParser;
using PromPP::WAL::hashdex::scraper::PrometheusScraper;

void BenchmarkParser(benchmark::State& state) {
  constexpr auto get_file_name = [] -> std::string {
    if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
      return context->operator[]("scraper_file");
    }

    return {};
  };

  std::ifstream t(get_file_name());
  std::string str((std::istreambuf_iterator(t)), std::istreambuf_iterator<char>());

  PrometheusParser parser;

  for ([[maybe_unused]] auto _ : state) {
    parser.tokenizer().tokenize(str);
    while (parser.tokenizer().next() != PromPP::Prometheus::textparse::Token::kEOF) {
      benchmark::DoNotOptimize(parser);
    }
  }
}

void BenchmarkScraperParse(benchmark::State& state) {
  constexpr auto get_file_name = [] -> std::string {
    if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
      return context->operator[]("scraper_file");
    }

    return {};
  };

  std::ifstream t(get_file_name());
  std::string str((std::istreambuf_iterator(t)), std::istreambuf_iterator<char>());

  for ([[maybe_unused]] auto _ : state) {
    PrometheusScraper scraper;
    auto tmp_str = str;
    std::ignore = scraper.parse(tmp_str, 0);
  }

  {
    PrometheusScraper scraper;
    auto tmp_str = str;
    std::ignore = scraper.parse(tmp_str, 0);
    state.counters["Alloc"] = benchmark::Counter(scraper.allocated_memory(), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);
  }
}

void BenchmarkScraperRead(benchmark::State& state) {
  constexpr auto get_file_name = [] -> std::string {
    if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
      return context->operator[]("scraper_file");
    }

    return {};
  };

  std::ifstream t(get_file_name());
  std::string str((std::istreambuf_iterator(t)), std::istreambuf_iterator<char>());

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

BENCHMARK(BenchmarkParser)->ComputeStatistics("min", [](const std::vector<double>& v) { return *std::ranges::min_element(v); });
BENCHMARK(BenchmarkScraperParse)->ComputeStatistics("min", [](const std::vector<double>& v) { return *std::ranges::min_element(v); });
BENCHMARK(BenchmarkScraperRead)->ComputeStatistics("min", [](const std::vector<double>& v) { return *std::ranges::min_element(v); });

}  // namespace
