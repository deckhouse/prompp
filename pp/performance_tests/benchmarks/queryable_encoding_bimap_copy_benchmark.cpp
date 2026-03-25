// ./bazel-bin/performance_tests/benchmarks/copy_lss --benchmark_context=wal_file="performance_tests/test_data/new/lss_real" --benchmark_counters_tabular=true
// --benchmark_repetitions=25
#include <benchmark/benchmark.h>

#include <algorithm>
#include <chrono>
#include <fstream>
#include <iostream>
#include <string>
#include <vector>

#include "primitives/snug_composites.h"
#include "profiling/profiling.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"

namespace {
using Lss =
    series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, BareBones::Vector, series_index::trie::CedarTrie>;

template <class DecodingTable, class SortingIndex, class SeriesIds, class QueryableEncodingBimap, class LsIdVector>
using LssCopier = series_index::QueryableEncodingBimapCopier<DecodingTable, SortingIndex, SeriesIds, QueryableEncodingBimap, LsIdVector>;

std::string GetWalFileName() {
  if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
    return context->operator[]("wal_file");
  }
  return {};
}

void mark_all_series_as_added(const std::shared_ptr<Lss>& lss) {
  for (auto label_set : *lss) {
    lss->find_or_emplace(label_set);
  }
}

std::shared_ptr<Lss> LoadLssFromFile() {
  const auto file_name = GetWalFileName();
  auto lss = std::make_shared<Lss>();

  std::ifstream istrm(file_name, std::ios::binary);
  istrm >> *lss;
  mark_all_series_as_added(lss);
  return lss;
}

std::vector<double> copy_added_series_times;
std::vector<double> copy_ls_id_set_times;
std::vector<double> build_trie_index_times;
std::vector<double> build_ls_id_hashset_times;
std::vector<double> build_reverse_index_times;

void BM_CopyAllStepsWithTiming(benchmark::State& state) {
  ZoneScoped;
  using std::chrono::nanoseconds;
  using std::chrono::steady_clock;

  static auto lss = LoadLssFromFile();
  lss->build_deferred_indexes();

  for ([[maybe_unused]] auto _ : state) {
    Lss lss_copy;
    BareBones::Vector<uint32_t> dst_src_ids_mapping;
    LssCopier copier(*lss, lss->sorting_index(), lss->added_series(), lss_copy, dst_src_ids_mapping);

    {
      auto start = steady_clock::now();
      copier.copy_added_series();
      auto end = steady_clock::now();
      copy_added_series_times.push_back(duration_cast<nanoseconds>(end - start).count());
    }

    {
      auto start = steady_clock::now();
      copier.copy_ls_id_set();
      auto end = steady_clock::now();
      copy_ls_id_set_times.push_back(duration_cast<nanoseconds>(end - start).count());
    }

    {
      auto start = steady_clock::now();
      copier.build_trie_index();
      auto end = steady_clock::now();
      build_trie_index_times.push_back(duration_cast<nanoseconds>(end - start).count());
    }

    {
      auto start = steady_clock::now();
      copier.build_ls_id_hashset();
      auto end = steady_clock::now();
      build_ls_id_hashset_times.push_back(duration_cast<nanoseconds>(end - start).count());
    }

    {
      auto start = steady_clock::now();
      copier.build_reverse_index();
      auto end = steady_clock::now();
      build_reverse_index_times.push_back(duration_cast<nanoseconds>(end - start).count());
    }
  }

  benchmark::DoNotOptimize(copy_added_series_times);
  benchmark::DoNotOptimize(copy_ls_id_set_times);
  benchmark::DoNotOptimize(build_trie_index_times);
  benchmark::DoNotOptimize(build_ls_id_hashset_times);
  benchmark::DoNotOptimize(build_reverse_index_times);
}

BENCHMARK(BM_CopyAllStepsWithTiming);

uint64_t Min(const std::vector<double>& v) {
  return v.empty() ? 0uz : static_cast<uint64_t>(*std::ranges::min_element(v));
}

void PrintMinStats() {
  std::cout << "\n=== Min method timings (ns) ===\n";

  constexpr int words_width = 20;
  constexpr int numbers_width = 10;

  std::cout << std::left << std::setw(words_width) << "copy_added_series"
            << ": " << std::right << std::setw(numbers_width) << Min(copy_added_series_times) << '\n';

  std::cout << std::left << std::setw(words_width) << "copy_ls_id_set"
            << ": " << std::right << std::setw(numbers_width) << Min(copy_ls_id_set_times) << '\n';

  std::cout << std::left << std::setw(words_width) << "build_trie_index"
            << ": " << std::right << std::setw(numbers_width) << Min(build_trie_index_times) << '\n';

  std::cout << std::left << std::setw(words_width) << "build_ls_id_hashset"
            << ": " << std::right << std::setw(numbers_width) << Min(build_ls_id_hashset_times) << '\n';

  std::cout << std::left << std::setw(words_width) << "build_reverse_index"
            << ": " << std::right << std::setw(numbers_width) << Min(build_reverse_index_times) << '\n';

  std::cout << "-------------------------------\n";

  std::cout << std::left << std::setw(words_width) << "total sum"
            << ": " << std::right << std::setw(numbers_width)
            << (Min(copy_added_series_times) + Min(copy_ls_id_set_times) + Min(build_trie_index_times) + Min(build_ls_id_hashset_times) +
                Min(build_reverse_index_times))
            << '\n';

  std::cout << "===============================\n";
}

}  // namespace

int main(int argc, char** argv) {
  ::benchmark::Initialize(&argc, argv);
  ::benchmark::RunSpecifiedBenchmarks();
  PrintMinStats();
}
