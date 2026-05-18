#include <benchmark/benchmark.h>

#include <fstream>

#include "bare_bones/vector.h"
#include "benchmark/statistic.h"
#include "profiling/profiling.h"
#include "series_index/prometheus/tsdb/index/index_write_context.h"
#include "series_index/queryable_encoding_bimap.h"

namespace {

using Lss = series_index::QueryableEncodingBimap<BareBones::Vector>;
using LssCopier =
    series_index::QueryableEncodingBimapCopier<Lss, typename Lss::SortingIndexBuilder::Index, BareBones::Bitset, Lss, BareBones::Vector<uint32_t>>;

std::string get_lss_file() {
  if (auto* ctx = benchmark::internal::GetGlobalContext(); ctx != nullptr) {
    return ctx->operator[]("lss_file");
  }
  return {};
}

void mark_all_series_as_added(Lss& lss) {
  for (const auto& label_set : lss) {
    lss.find_or_emplace(label_set);
  }
}

const Lss& get_lss_no_shrink() {
  static Lss lss;
  if (lss.items_count() == 0) {
    const auto path = get_lss_file();
    if (!path.empty()) {
      std::ifstream in(path, std::ios::binary);
      if (in.is_open()) {
        in >> lss;
        mark_all_series_as_added(lss);
        lss.build_deferred_indexes();
      }
    }
  }
  return lss;
}

const Lss& get_lss_after_shrink() {
  struct ShrunkState {
    Lss lss;
    Lss snapshot_copy;
  };

  static ShrunkState state;
  static bool initialized = false;
  if (!initialized) {
    const auto& source = get_lss_no_shrink();
    const uint32_t total = static_cast<uint32_t>(source.next_item_index());
    for (uint32_t i = 0; i < total; ++i) {
      state.lss.find_or_emplace(source[i]);
    }
    state.lss.build_deferred_indexes();

    const uint32_t shrink_boundary = total;
    BareBones::Vector<uint32_t> dst_src_ids_mapping;
    LssCopier copier(state.lss, state.lss.sorting_index(), state.lss.added_series(), state.snapshot_copy, dst_src_ids_mapping);
    copier.copy_added_series_and_build_indexes();

    state.lss.set_pending_shrink_boundary(shrink_boundary);
    state.lss.finalize_copy_and_shrink(state.snapshot_copy, dst_src_ids_mapping);
    initialized = true;
  }
  return state.lss;
}

void IndexWriteContextNoShrink(benchmark::State& state) {
  ZoneScoped;
  const auto& lss = get_lss_no_shrink();
  for ([[maybe_unused]] auto _ : state) {
    auto context = series_index::prometheus::tsdb::index::IndexWriteContext<Lss>{lss};
    benchmark::DoNotOptimize(context);
  }
  state.SetItemsProcessed(static_cast<int64_t>(lss.next_item_index()) * state.iterations());
}

void IndexWriteContextAfterShrink(benchmark::State& state) {
  ZoneScoped;
  const auto& lss = get_lss_after_shrink();
  for ([[maybe_unused]] auto _ : state) {
    auto context = series_index::prometheus::tsdb::index::IndexWriteContext<Lss>{lss};
    benchmark::DoNotOptimize(context);
  }
  state.SetItemsProcessed(static_cast<int64_t>(lss.next_item_index()) * state.iterations());
}

}  // namespace

BENCHMARK(IndexWriteContextNoShrink)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK(IndexWriteContextAfterShrink)->ComputeStatistics("min", benchmark::min_time);
