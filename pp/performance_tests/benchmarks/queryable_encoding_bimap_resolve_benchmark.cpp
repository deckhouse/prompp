#include <benchmark/benchmark.h>

#include <fstream>
#include <memory>

#include "bare_bones/vector.h"
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

void mark_all_series_as_added(const std::shared_ptr<Lss>& lss) {
  for (const auto& label_set : *lss) {
    lss->find_or_emplace(label_set);
  }
}

std::shared_ptr<Lss> load_lss_from_file() {
  const auto path = get_lss_file();
  auto lss = std::make_shared<Lss>();
  if (path.empty()) {
    return lss;
  }
  std::ifstream in(path, std::ios::binary);
  if (!in.is_open()) {
    return lss;
  }
  in >> *lss;
  mark_all_series_as_added(lss);
  lss->build_deferred_indexes();
  return lss;
}

std::shared_ptr<Lss> get_lss_no_shrink() {
  static auto lss = load_lss_from_file();
  return lss;
}

struct ShrunkState {
  std::shared_ptr<Lss> lss;
  std::unique_ptr<Lss> copy;
  BareBones::Vector<uint32_t> old_to_new;
};

std::shared_ptr<Lss> get_lss_after_shrink() {
  static const std::shared_ptr<ShrunkState> state = []() {
    auto s = std::make_shared<ShrunkState>();
    const auto source = get_lss_no_shrink();
    const uint32_t total = static_cast<uint32_t>(source->next_item_index());
    const uint32_t copy_count = static_cast<uint32_t>((static_cast<uint64_t>(total) * 90) / 100);

    s->lss = std::make_shared<Lss>();
    Lss& lss = *s->lss;
    for (uint32_t i = 0; i < copy_count; ++i) {
      lss.find_or_emplace((*source)[i]);
    }
    for (uint32_t i = copy_count; i < total; ++i) {
      lss.find_or_emplace((*source)[i]);
    }
    lss.build_deferred_indexes();

    const uint32_t shrink_boundary = copy_count;
    s->copy = std::make_unique<Lss>();
    BareBones::Vector<uint32_t> dst_src_ids_mapping;
    LssCopier copier(lss, lss.sorting_index(), lss.added_series(), *s->copy, dst_src_ids_mapping);
    copier.copy_added_series_and_build_indexes();

    series_index::invert_copy_mapping(dst_src_ids_mapping, copy_count, s->old_to_new);
    lss.fill_added_series_mapping(shrink_boundary, *s->copy, s->old_to_new, source->added_series());
    lss.finalize_copy_and_shrink(shrink_boundary, *s->copy, s->old_to_new);
    return s;
  }();
  return state->lss;
}

static void run_resolve_loop(const std::shared_ptr<Lss>& lss, benchmark::State& state) {
  const auto total = static_cast<uint32_t>(lss->next_item_index());
  for ([[maybe_unused]] auto _ : state) {
    for (uint32_t id = 0; id < total; ++id) {
      benchmark::DoNotOptimize((*lss)[id]);
    }
  }
  state.SetItemsProcessed(static_cast<int64_t>(total) * state.iterations());
  state.counters["per_resolve"] = benchmark::Counter(static_cast<double>(total), benchmark::Counter::kIsIterationInvariantRate | benchmark::Counter::kInvert);
}

void BM_ResolveNoShrink(benchmark::State& state) {
  ZoneScoped;
  run_resolve_loop(get_lss_no_shrink(), state);
}

void BM_ResolveAfterShrink(benchmark::State& state) {
  ZoneScoped;
  run_resolve_loop(get_lss_after_shrink(), state);
}

double min_value(const std::vector<double>& v) noexcept {
  return v.empty() ? 0.0 : *std::ranges::min_element(v);
}

}  // namespace

BENCHMARK(BM_ResolveNoShrink)->ComputeStatistics("min", min_value);
BENCHMARK(BM_ResolveAfterShrink)->ComputeStatistics("min", min_value);
