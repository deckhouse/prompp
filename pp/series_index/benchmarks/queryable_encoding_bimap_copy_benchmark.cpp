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

using Lss = series_index::QueryableEncodingBimap<BareBones::Vector>;

template <class DecodingTable, class SortingIndex, class SeriesIds, class QueryableEncodingBimap, class LsIdVector>
using LssCopier = series_index::QueryableEncodingBimapCopier<DecodingTable, SortingIndex, SeriesIds, QueryableEncodingBimap, LsIdVector>;

std::string get_lss_file_name() {
  if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
    return context->operator[]("lss_file");
  }
  return {};
}

void mark_all_series_as_added(Lss& lss) {
  for (auto label_set : lss) {
    lss.find_or_emplace(label_set);
  }
}

const Lss& load_lss_from_file() {
  static Lss lss;
  if (lss.size() == 0) [[unlikely]] {
    const auto file_name = get_lss_file_name();

    std::ifstream istrm(file_name, std::ios::binary);
    istrm >> lss;
    mark_all_series_as_added(lss);
    lss.build_deferred_indexes();
  }

  return lss;
}

struct CopyPartsTimings {
  std::chrono::nanoseconds copy_added_series{};
  std::chrono::nanoseconds copy_ls_id_set{};
  std::chrono::nanoseconds build_trie_index{};
  std::chrono::nanoseconds build_ls_id_hashset{};
  std::chrono::nanoseconds build_reverse_index{};

  PROMPP_ALWAYS_INLINE std::chrono::nanoseconds total() const noexcept {
    return copy_added_series + copy_ls_id_set + build_trie_index + build_ls_id_hashset + build_reverse_index;
  }
};

std::vector<CopyPartsTimings> copy_parts_timings;

void CopyAllStepsWithTiming(benchmark::State& state) {
  ZoneScoped;
  using std::chrono::nanoseconds;
  using std::chrono::steady_clock;

  auto& lss = load_lss_from_file();

  auto& timings = copy_parts_timings.emplace_back();

  for ([[maybe_unused]] auto _ : state) {
    Lss lss_copy;
    BareBones::Vector<uint32_t> dst_src_ids_mapping;
    LssCopier copier(lss, lss.sorting_index(), lss.added_series(), lss_copy, dst_src_ids_mapping);

    {
      auto start = steady_clock::now();
      copier.copy_added_series();
      timings.copy_added_series += steady_clock::now() - start;
    }

    {
      auto start = steady_clock::now();
      copier.copy_ls_id_set();
      timings.copy_ls_id_set += steady_clock::now() - start;
    }

    {
      auto start = steady_clock::now();
      copier.build_trie_index();
      timings.build_trie_index += steady_clock::now() - start;
    }

    {
      auto start = steady_clock::now();
      copier.build_ls_id_hashset();
      timings.build_ls_id_hashset += steady_clock::now() - start;
    }

    {
      auto start = steady_clock::now();
      copier.build_reverse_index();
      timings.build_reverse_index += steady_clock::now() - start;
    }
  }
}

#define MIN_BY_FIELD(field)                                                                                                                                   \
  [](const auto&) {                                                                                                                                           \
    return std::chrono::duration<double>(std::ranges::min_element(copy_parts_timings, [](const auto& a, const auto& b) { return a.field < b.field; })->field) \
        .count();                                                                                                                                             \
  }

BENCHMARK(CopyAllStepsWithTiming)
    // We explicitly set Iterations(1) for the correct calculation benchmark times
    ->Iterations(1)
    ->ComputeStatistics("min_copy_added_series", MIN_BY_FIELD(copy_added_series))
    ->ComputeStatistics("min_copy_ls_id_set", MIN_BY_FIELD(copy_ls_id_set))
    ->ComputeStatistics("min_build_trie_index", MIN_BY_FIELD(build_trie_index))
    ->ComputeStatistics("min_build_ls_id_hashset", MIN_BY_FIELD(build_ls_id_hashset))
    ->ComputeStatistics("min_build_reverse_index", MIN_BY_FIELD(build_reverse_index))
    ->ComputeStatistics("min_total", MIN_BY_FIELD(total()));

#undef MIN_BY_FIELD

}  // namespace
