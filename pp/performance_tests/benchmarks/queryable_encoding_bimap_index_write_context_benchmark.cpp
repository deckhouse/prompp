#include <benchmark/benchmark.h>

#include <algorithm>
#include <cassert>
#include <fstream>
#include <memory>
#include <vector>

#include "bare_bones/vector.h"
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

void assert_added_series_suffix_marked(const Lss& lss, uint32_t begin_id) {
  const auto& added = lss.added_series();
  const uint32_t end_id = static_cast<uint32_t>(lss.max_item_index());
  assert(begin_id <= end_id);
  for (uint32_t id = begin_id; id < end_id; ++id) {
    assert(id < added.size());
    assert(added[id]);
  }
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

std::shared_ptr<Lss> get_lss_after_shrink() {
  struct ShrunkState {
    std::shared_ptr<Lss> lss;
    std::unique_ptr<Lss> snapshot_copy;
  };

  static const std::shared_ptr<ShrunkState> state = []() {
    auto s = std::make_shared<ShrunkState>();
    auto source = get_lss_no_shrink();
    s->lss = std::make_shared<Lss>();
    auto& lss = *s->lss;
    const uint32_t total = static_cast<uint32_t>(source->max_item_index());
    for (uint32_t i = 0; i < total; ++i) {
      lss.find_or_emplace((*source)[i]);
    }
    lss.build_deferred_indexes();

    const uint32_t shrink_boundary = total;
    BareBones::Vector<uint32_t> dst_src_ids_mapping;
    s->snapshot_copy = std::make_unique<Lss>();
    LssCopier copier(lss, lss.sorting_index(), lss.added_series(), *s->snapshot_copy, dst_src_ids_mapping);
    copier.copy_added_series_and_build_indexes();

    assert_added_series_suffix_marked(lss, shrink_boundary);
    lss.set_pending_shrink_boundary(shrink_boundary);
    lss.finalize_copy_and_shrink(*s->snapshot_copy, dst_src_ids_mapping);
    assert_added_series_suffix_marked(lss, shrink_boundary);
    return s;
  }();
  return state->lss;
}

void BM_IndexWriteContextNoShrink(benchmark::State& state) {
  ZoneScoped;
  const auto lss = get_lss_no_shrink();
  for ([[maybe_unused]] auto _ : state) {
    auto context = series_index::prometheus::tsdb::index::IndexWriteContext<Lss>{*lss};
    benchmark::DoNotOptimize(context);
  }
  state.SetItemsProcessed(static_cast<int64_t>(lss->max_item_index()) * state.iterations());
}

void BM_IndexWriteContextAfterShrink(benchmark::State& state) {
  ZoneScoped;
  const auto lss = get_lss_after_shrink();
  for ([[maybe_unused]] auto _ : state) {
    auto context = series_index::prometheus::tsdb::index::IndexWriteContext<Lss>{*lss};
    benchmark::DoNotOptimize(context);
  }
  state.SetItemsProcessed(static_cast<int64_t>(lss->max_item_index()) * state.iterations());
}

double min_value(const std::vector<double>& v) noexcept {
  return v.empty() ? 0.0 : *std::ranges::min_element(v);
}

}  // namespace

BENCHMARK(BM_IndexWriteContextNoShrink)->ComputeStatistics("min", min_value);
BENCHMARK(BM_IndexWriteContextAfterShrink)->ComputeStatistics("min", min_value);
