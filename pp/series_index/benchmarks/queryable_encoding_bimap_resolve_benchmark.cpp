#include <benchmark/benchmark.h>

#include <fstream>

#include "bare_bones/vector.h"
#include "benchmark/statistic.h"
#include "profiling/profiling.h"
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

const Lss& get_lss_fixed_state() {
  static Lss lss;
  static bool initialized = false;
  if (!initialized) {
    const auto& source = get_lss_no_shrink();
    const uint32_t total = static_cast<uint32_t>(source.next_item_index());
    const uint32_t copy_count = static_cast<uint32_t>((static_cast<uint64_t>(total) * 90) / 100);

    for (uint32_t i = 0; i < copy_count; ++i) {
      lss.find_or_emplace(source[i]);
    }
    for (uint32_t i = copy_count; i < total; ++i) {
      lss.find_or_emplace(source[i]);
    }
    lss.build_deferred_indexes();

    lss.set_pending_shrink_boundary(copy_count);
    initialized = true;
  }
  return lss;
}

struct ShrunkState {
  Lss lss;
  Lss copy;
};

const Lss& get_lss_after_shrink() {
  static ShrunkState state;
  static bool initialized = false;
  if (!initialized) {
    const auto& source = get_lss_no_shrink();
    const uint32_t total = static_cast<uint32_t>(source.next_item_index());
    const uint32_t copy_count = static_cast<uint32_t>((static_cast<uint64_t>(total) * 90) / 100);

    for (uint32_t i = 0; i < copy_count; ++i) {
      state.lss.find_or_emplace(source[i]);
    }
    for (uint32_t i = copy_count; i < total; ++i) {
      state.lss.find_or_emplace(source[i]);
    }
    state.lss.build_deferred_indexes();

    const uint32_t shrink_boundary = copy_count;
    BareBones::Vector<uint32_t> dst_src_ids_mapping;
    LssCopier copier(state.lss, state.lss.sorting_index(), state.lss.added_series(), state.copy, dst_src_ids_mapping);
    copier.copy_added_series_and_build_indexes();

    state.lss.set_pending_shrink_boundary(shrink_boundary);
    state.lss.finalize_copy_and_shrink(state.copy, dst_src_ids_mapping);
    initialized = true;
  }
  return state.lss;
}

static void run_resolve_loop(const Lss& lss, benchmark::State& state) {
  const auto total = static_cast<uint32_t>(lss.next_item_index());
  for ([[maybe_unused]] auto _ : state) {
    for (uint32_t id = 0; id < total; ++id) {
      benchmark::DoNotOptimize(lss[id]);
    }
  }
  state.SetItemsProcessed(static_cast<int64_t>(total) * state.iterations());
  state.counters["per_resolve"] = benchmark::Counter(static_cast<double>(total), benchmark::Counter::kIsIterationInvariantRate | benchmark::Counter::kInvert);
}

void ResolveNoShrink(benchmark::State& state) {
  ZoneScoped;
  run_resolve_loop(get_lss_no_shrink(), state);
}

void ResolveAfterShrink(benchmark::State& state) {
  ZoneScoped;
  run_resolve_loop(get_lss_after_shrink(), state);
}

void ResolveFixedState(benchmark::State& state) {
  ZoneScoped;
  run_resolve_loop(get_lss_fixed_state(), state);
}

}  // namespace

BENCHMARK(ResolveNoShrink)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK(ResolveFixedState)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK(ResolveAfterShrink)->ComputeStatistics("min", benchmark::min_time);
