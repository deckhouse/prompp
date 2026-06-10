#include <benchmark/benchmark.h>

#include <algorithm>
#include <chrono>
#include <cstdint>
#include <fstream>
#include <numeric>
#include <vector>

#include "bare_bones/vector.h"
#include "primitives/snug_composites.h"
#include "profiling/profiling.h"
#include "series_index/queryable_encoding_bimap.h"

namespace {

template <class T>
using DefaultSharedVector = BareBones::SharedVector<T, BareBones::DefaultReallocator>;
using Lss = series_index::QueryableEncodingBimap<DefaultSharedVector>;
template <class T>
using DefaultSharedSpan = BareBones::SharedSpan<T, BareBones::DefaultReallocator>;
using ReadonlyLss = PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<DefaultSharedSpan>;
using LssCopier =
    series_index::QueryableEncodingBimapCopier<Lss, typename Lss::SortingIndexBuilder::Index, BareBones::Bitset, Lss, BareBones::Vector<uint32_t>>;

std::string get_lss_file() {
  if (auto* ctx = benchmark::internal::GetGlobalContext(); ctx != nullptr) {
    return ctx->operator[]("lss_file");
  }
  return {};
}

void load_lss_from_file(Lss& lss) {
  std::ifstream in(get_lss_file(), std::ios::binary);
  in >> lss;
}

uint32_t compute_boundary(uint32_t total, uint32_t boundary_percent) {
  const uint64_t scaled = static_cast<uint64_t>(total) * boundary_percent;
  return static_cast<uint32_t>(scaled / 100U);
}

uint32_t compute_active_count(uint32_t boundary, uint32_t hole_percent) {
  const uint64_t scaled = static_cast<uint64_t>(boundary) * hole_percent;
  const auto hole_count = static_cast<uint32_t>(scaled / 100U);
  return boundary - hole_count;
}

uint64_t deterministic_key(uint32_t value) {
  uint64_t x = value + 0x9E3779B97F4A7C15ULL;
  x = (x ^ (x >> 30U)) * 0xBF58476D1CE4E5B9ULL;
  x = (x ^ (x >> 27U)) * 0x94D049BB133111EBULL;
  return x ^ (x >> 31U);
}

void mark_random_active_series(Lss& lss, uint32_t boundary, uint32_t active_count) {
  const_cast<BareBones::Bitset&>(lss.added_series()).resize(boundary);
  std::vector<uint32_t> series_ids(boundary);
  std::iota(series_ids.begin(), series_ids.end(), 0U);
  std::sort(series_ids.begin(), series_ids.end(), [](uint32_t lhs, uint32_t rhs) { return deterministic_key(lhs) < deterministic_key(rhs); });

  for (uint32_t index = 0; index < active_count; ++index) {
    std::ignore = lss.find_or_emplace(lss[series_ids[index]]);
  }
}

void set_common_counters(benchmark::State& state, uint32_t total, uint32_t boundary, uint32_t hole_percent, uint32_t active_count) {
  state.counters["total_series"] = static_cast<double>(total);
  state.counters["boundary"] = static_cast<double>(boundary);
  state.counters["hole_percent"] = static_cast<double>(hole_percent);
  state.counters["active_count"] = static_cast<double>(active_count);
  state.counters["active_ratio"] = static_cast<double>(active_count) / static_cast<double>(boundary);
  state.counters["hole_ratio"] = 1.0 - static_cast<double>(active_count) / static_cast<double>(boundary);
}

struct FreezePartsTimings {
  std::chrono::nanoseconds copy_added_series_and_build_indexes{};
  std::chrono::nanoseconds set_pending_shrink_boundary{};
  std::chrono::nanoseconds lazy_sorting_index_rebuild{};
  std::chrono::nanoseconds finalize_copy_and_shrink{};

  PROMPP_ALWAYS_INLINE std::chrono::nanoseconds total() const noexcept {
    return copy_added_series_and_build_indexes + set_pending_shrink_boundary + lazy_sorting_index_rebuild + finalize_copy_and_shrink;
  }
};

std::vector<FreezePartsTimings> freeze_timings;
int64_t current_boundary_percent = -1;
int64_t current_hole_percent = -1;

void reset_timings_if_needed(benchmark::State& state) {
  if (current_boundary_percent == state.range(0) && current_hole_percent == state.range(1)) {
    return;
  }

  freeze_timings.clear();
  current_boundary_percent = state.range(0);
  current_hole_percent = state.range(1);
}

void FreezeAllStepsWithTiming(benchmark::State& state) {
  ZoneScoped;
  using std::chrono::steady_clock;
  reset_timings_if_needed(state);

  Lss sample_lss;
  load_lss_from_file(sample_lss);
  const uint32_t total = sample_lss.next_item_index();
  const uint32_t boundary = compute_boundary(total, static_cast<uint32_t>(state.range(0)));
  const uint32_t hole_percent = static_cast<uint32_t>(state.range(1));
  const uint32_t active_count = compute_active_count(boundary, hole_percent);
  set_common_counters(state, total, boundary, hole_percent, active_count);

  auto& timings = freeze_timings.emplace_back();

  for ([[maybe_unused]] auto _ : state) {
    state.PauseTiming();
    Lss lss;
    load_lss_from_file(lss);
    mark_random_active_series(lss, boundary, active_count);
    lss.build_deferred_indexes();
    Lss copy;
    BareBones::Vector<uint32_t> dst_src_ids_mapping;
    state.ResumeTiming();

    LssCopier copier(lss, lss.sorting_index(), lss.added_series(), copy, dst_src_ids_mapping);
    {
      const auto start = steady_clock::now();
      copier.copy_added_series_and_build_indexes();
      timings.copy_added_series_and_build_indexes += steady_clock::now() - start;
    }

    {
      const auto start = steady_clock::now();
      lss.set_pending_shrink_boundary(boundary);
      timings.set_pending_shrink_boundary += steady_clock::now() - start;
    }

    {
      const auto start = steady_clock::now();
      lss.build_deferred_indexes();
      timings.lazy_sorting_index_rebuild += steady_clock::now() - start;
    }

    {
      const auto start = steady_clock::now();
      const ReadonlyLss resolve_snapshot(copy);
      lss.finalize_copy_and_shrink(resolve_snapshot, dst_src_ids_mapping);
      timings.finalize_copy_and_shrink += steady_clock::now() - start;
    }

    benchmark::DoNotOptimize(lss);
  }

  state.SetItemsProcessed(static_cast<int64_t>(boundary) * state.iterations());
}

void ApplyFreezeArgs(benchmark::Benchmark* benchmark) {
  for (const int boundary_percent : {10, 25, 50, 90}) {
    for (const int hole_percent : {10, 50, 70}) {
      benchmark->Args({boundary_percent, hole_percent});
    }
  }
}

}  // namespace

#define MIN_BY_FIELD(field)                                                                                                     \
  [](const auto&) {                                                                                                             \
    return std::chrono::duration<double, std::milli>(                                                                           \
               std::ranges::min_element(freeze_timings, [](const auto& a, const auto& b) { return a.field < b.field; })->field) \
        .count();                                                                                                               \
  }

BENCHMARK(FreezeAllStepsWithTiming)
    ->Iterations(1)
    ->Apply(ApplyFreezeArgs)
    ->ComputeStatistics("min_copy_ms", MIN_BY_FIELD(copy_added_series_and_build_indexes))
    ->ComputeStatistics("min_set_pending_ms", MIN_BY_FIELD(set_pending_shrink_boundary))
    ->ComputeStatistics("min_lazy_sorting_rebuild_ms", MIN_BY_FIELD(lazy_sorting_index_rebuild))
    ->ComputeStatistics("min_finalize_ms", MIN_BY_FIELD(finalize_copy_and_shrink))
    ->ComputeStatistics("min_total_ms", MIN_BY_FIELD(total()));

#undef MIN_BY_FIELD
