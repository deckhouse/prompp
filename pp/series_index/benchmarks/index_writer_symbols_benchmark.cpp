#include <benchmark/benchmark.h>

#include <algorithm>
#include <cstdint>
#include <fstream>
#include <ios>
#include <numeric>
#include <random>
#include <sstream>
#include <string>
#include <string_view>
#include <vector>

#include "bare_bones/bitset.h"
#include "bare_bones/vector.h"
#include "benchmark/statistic.h"
#include "primitives/snug_composites.h"
#include "series_index/prometheus/tsdb/index/index_write_context.h"
#include "series_index/prometheus/tsdb/index/index_writer.h"
#include "series_index/queryable_encoding_bimap.h"

#include "profiling/profiling.h"

namespace {

template <class T>
using DefaultSharedVector = BareBones::SharedVector<T, BareBones::DefaultReallocator>;
template <class T>
using DefaultSharedSpan = BareBones::SharedSpan<T, BareBones::DefaultReallocator>;

using Lss = series_index::QueryableEncodingBimap<DefaultSharedVector>;
using ReadonlyLss = PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<DefaultSharedSpan>;
using IndexWriteContext = series_index::prometheus::tsdb::index::IndexWriteContext<Lss>;
using IndexWriter = series_index::prometheus::tsdb::index::IndexWriter<Lss, std::ostringstream>;
using LssCopier =
    series_index::QueryableEncodingBimapCopier<Lss, typename Lss::SortingIndexBuilder::Index, BareBones::Bitset, Lss, BareBones::Vector<uint32_t>>;

struct ShrunkState {
  Lss lss;
  Lss snapshot_copy;
};

struct ActiveSeries {
  uint32_t shrink_boundary{};
  BareBones::Bitset snapshot_series;
};

std::string get_lss_file() {
  if (auto* ctx = benchmark::internal::GetGlobalContext(); ctx != nullptr) {
    return ctx->operator[]("lss_file");
  }
  return {};
}

void load_lss_from_file(Lss& lss) {
  const auto path = get_lss_file();
  if (path.empty()) {
    return;
  }

  std::ifstream in(path, std::ios::binary);
  if (in.is_open()) {
    in >> lss;
  }
}

ActiveSeries mark_active_series_for_shrink(Lss& lss) {
  const uint32_t total = static_cast<uint32_t>(lss.next_item_index());
  const uint32_t boundary = static_cast<uint32_t>((static_cast<uint64_t>(total) * 90U) / 100U);
  const uint32_t active_before_boundary = static_cast<uint32_t>((static_cast<uint64_t>(boundary) * 70U) / 100U);

  std::vector<uint32_t> ids(boundary);
  std::iota(ids.begin(), ids.end(), 0U);
  std::mt19937 rng{13};
  std::shuffle(ids.begin(), ids.end(), rng);

  ActiveSeries active_series;
  active_series.shrink_boundary = boundary;
  for (uint32_t i = 0; i < active_before_boundary; ++i) {
    lss.mark_active(ids[i]);
    active_series.snapshot_series.set(ids[i]);
  }

  if (boundary != 0) {
    lss.mark_active(boundary - 1);
    active_series.snapshot_series.set(boundary - 1);
  }

  for (uint32_t id = boundary; id < total; ++id) {
    lss.mark_active(id);
  }

  return active_series;
}

void build_loaded_lss(Lss& lss) {
  load_lss_from_file(lss);
  lss.build_deferred_indexes();
}

uint32_t count_symbols(const Lss& lss) {
  uint32_t symbols_count = 0;
  const IndexWriteContext context{lss};
  context.for_each_symbol([&symbols_count](uint32_t, std::string_view) { ++symbols_count; });
  return symbols_count;
}

void set_symbol_counters(benchmark::State& state, uint32_t symbols_count) {
  state.SetItemsProcessed(static_cast<int64_t>(symbols_count) * state.iterations());
  state.counters["unique_symbols"] = static_cast<double>(symbols_count);
  state.counters["per_symbol"] =
      benchmark::Counter(static_cast<double>(symbols_count), benchmark::Counter::kIsIterationInvariantRate | benchmark::Counter::kInvert);
}

const Lss& get_lss_no_shrink() {
  static Lss lss;
  static bool initialized = false;
  if (!initialized) {
    build_loaded_lss(lss);
    initialized = true;
  }
  return lss;
}

const Lss& get_lss_fixed_state() {
  static Lss lss;
  static bool initialized = false;
  if (!initialized) {
    build_loaded_lss(lss);
    const auto active_series = mark_active_series_for_shrink(lss);
    lss.set_pending_shrink_boundary(active_series.shrink_boundary);
    initialized = true;
  }
  return lss;
}

const Lss& get_lss_after_shrink() {
  static ShrunkState state;
  static bool initialized = false;
  if (!initialized) {
    build_loaded_lss(state.lss);
    const auto active_series = mark_active_series_for_shrink(state.lss);

    BareBones::Vector<uint32_t> dst_src_ids_mapping;
    LssCopier copier(state.lss, state.lss.sorting_index(), active_series.snapshot_series, state.snapshot_copy, dst_src_ids_mapping);
    copier.copy_added_series_and_build_indexes();

    state.lss.set_pending_shrink_boundary(active_series.shrink_boundary);
    const ReadonlyLss resolve_snapshot(state.snapshot_copy);
    state.lss.finalize_copy_and_shrink(resolve_snapshot, dst_src_ids_mapping);
    initialized = true;
  }
  return state.lss;
}

using LssProvider = const Lss& (*)();

inline void run_index_writer_symbols(benchmark::State& state, LssProvider get_lss) {
  const auto& lss = get_lss();

  const uint32_t symbols_count = count_symbols(lss);
  std::ostringstream stream;
  IndexWriter writer{lss};

  for ([[maybe_unused]] auto _ : state) {
    state.PauseTiming();
    stream.str({});
    stream.clear();
    state.ResumeTiming();

    {
      ZoneScopedN("IndexWriterWriteSymbolsLoop");
      writer.write_symbols(stream);
    }
    benchmark::DoNotOptimize(stream.view().size());
  }

  set_symbol_counters(state, symbols_count);
}

void IndexWriterSymbolsNoShrink(benchmark::State& state) {
  ZoneScopedN("IndexWriterSymbolsNoShrink");
  run_index_writer_symbols(state, get_lss_no_shrink);
}

void IndexWriterSymbolsFixedState(benchmark::State& state) {
  ZoneScopedN("IndexWriterSymbolsFixedState");
  run_index_writer_symbols(state, get_lss_fixed_state);
}

void IndexWriterSymbolsAfterShrink(benchmark::State& state) {
  ZoneScopedN("IndexWriterSymbolsAfterShrink");
  run_index_writer_symbols(state, get_lss_after_shrink);
}

}  // namespace

BENCHMARK(IndexWriterSymbolsNoShrink)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK(IndexWriterSymbolsFixedState)->ComputeStatistics("min", benchmark::min_time);
BENCHMARK(IndexWriterSymbolsAfterShrink)->ComputeStatistics("min", benchmark::min_time);
