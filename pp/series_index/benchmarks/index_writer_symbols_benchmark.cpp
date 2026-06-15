#include <benchmark/benchmark.h>

#include <chrono>
#include <fstream>
#include <numeric>
#include <random>
#include <sstream>
#include <string>
#include <vector>

#include "bare_bones/bitset.h"
#include "bare_bones/vector.h"
#include "primitives/snug_composites.h"
#include "profiling/profiling.h"
#include "series_index/prometheus/tsdb/index/index_writer.h"
#include "series_index/queryable_encoding_bimap.h"

namespace {

template <class T>
using DefaultSharedVector = BareBones::SharedVector<T, BareBones::DefaultReallocator>;
template <class T>
using DefaultSharedSpan = BareBones::SharedSpan<T, BareBones::DefaultReallocator>;

using Lss = series_index::QueryableEncodingBimap<DefaultSharedVector>;
using ReadonlyLss = PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<DefaultSharedSpan>;
using IndexWriter = series_index::prometheus::tsdb::index::IndexWriter<Lss, std::ostringstream>;
using LssCopier =
    series_index::QueryableEncodingBimapCopier<Lss, typename Lss::SortingIndexBuilder::Index, BareBones::Bitset, Lss, BareBones::Vector<uint32_t>>;
using ChunkMetadata = series_index::prometheus::tsdb::index::ChunkMetadata;

// Matches go/storage/block/block.go WriteRestTo postings batch size.
constexpr uint32_t kPostingsBatchSize = 1U << 20U;

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

void mark_all_series_as_added(Lss& lss) {
  for (const auto& label_set : lss) {
    lss.find_or_emplace(label_set);
  }
}

void load_lss_from_file(Lss& lss) {
  const auto path = get_lss_file();
  if (path.empty()) {
    return;
  }

  std::ifstream in(path, std::ios::binary);
  if (in.is_open()) {
    in >> lss;
    mark_all_series_as_added(lss);
    lss.build_deferred_indexes();
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

const Lss& get_lss_no_shrink() {
  static Lss lss;
  if (lss.items_count() == 0) {
    load_lss_from_file(lss);
  }
  return lss;
}

const Lss& get_lss_fixed_state() {
  static Lss lss;
  if (lss.items_count() == 0) {
    load_lss_from_file(lss);
    const auto active_series = mark_active_series_for_shrink(lss);
    lss.set_pending_shrink_boundary(active_series.shrink_boundary);
  }
  return lss;
}

const Lss& get_lss_after_shrink() {
  static ShrunkState state;
  static bool initialized = false;
  if (!initialized) {
    load_lss_from_file(state.lss);
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

const std::vector<ChunkMetadata>& representative_chunks() {
  static const std::vector<ChunkMetadata> chunks{
      {.min_timestamp = 1000, .max_timestamp = 2000, .reference = 100},
      {.min_timestamp = 2001, .max_timestamp = 4000, .reference = 200},
  };
  return chunks;
}

uint32_t first_non_empty_series_id(const Lss& lss) {
  for (uint32_t ls_id = 0; ls_id < lss.next_item_index(); ++ls_id) {
    if (lss[ls_id].size() != 0) {
      return ls_id;
    }
  }
  return 0;
}

PROMPP_ALWAYS_INLINE void reset_stream(std::ostringstream& stream) {
  stream.str({});
  stream.clear();
}

void prepare_writer_before_series(IndexWriter& writer, std::ostringstream& stream) {
  reset_stream(stream);
  writer.write_header(stream);
  reset_stream(stream);
  writer.write_symbols(stream);
}

void prepare_writer_before_label_indices(IndexWriter& writer, std::ostringstream& stream, uint32_t ls_id, const std::vector<ChunkMetadata>& chunks) {
  prepare_writer_before_series(writer, stream);
  reset_stream(stream);
  writer.write_series(ls_id, chunks, stream);
}

void write_all_postings(IndexWriter& writer, std::ostringstream& stream) {
  do {
    reset_stream(stream);
    writer.write_postings(stream, kPostingsBatchSize);
  } while (writer.has_more_postings_data());
}

// One timed sample per entrypoint call (see entrypoint/index_writer.h).
struct IndexWriterCallTimings {
  std::chrono::nanoseconds write_header{};
  std::chrono::nanoseconds write_symbols_no_shrink{};
  std::chrono::nanoseconds write_symbols_fixed_state{};
  std::chrono::nanoseconds write_symbols_after_shrink{};
  std::chrono::nanoseconds write_next_series_batch{};
  std::chrono::nanoseconds write_label_indices{};
  std::chrono::nanoseconds write_next_postings_batch{};
  std::chrono::nanoseconds write_label_indices_table{};
  std::chrono::nanoseconds write_postings_table_offsets{};
  std::chrono::nanoseconds write_table_of_contents{};
};

std::vector<IndexWriterCallTimings> index_writer_call_timings;

void IndexWriterEntrypointCalls(benchmark::State& state) {
  ZoneScoped;
  using std::chrono::steady_clock;

  const auto& lss = get_lss_no_shrink();
  const auto& chunks = representative_chunks();
  const auto ls_id = first_non_empty_series_id(lss);

  auto& timings = index_writer_call_timings.emplace_back();

  for ([[maybe_unused]] auto _ : state) {
    {
      IndexWriter writer{lss};
      std::ostringstream stream;
      const auto start = steady_clock::now();
      writer.write_header(stream);
      timings.write_header += steady_clock::now() - start;
      benchmark::DoNotOptimize(stream.view().size());
    }

    {
      IndexWriter writer{get_lss_no_shrink()};
      std::ostringstream stream;
      const auto start = steady_clock::now();
      writer.write_symbols(stream);
      timings.write_symbols_no_shrink += steady_clock::now() - start;
      benchmark::DoNotOptimize(stream.view().size());
    }

    {
      IndexWriter writer{get_lss_fixed_state()};
      std::ostringstream stream;
      const auto start = steady_clock::now();
      writer.write_symbols(stream);
      timings.write_symbols_fixed_state += steady_clock::now() - start;
      benchmark::DoNotOptimize(stream.view().size());
    }

    {
      IndexWriter writer{get_lss_after_shrink()};
      std::ostringstream stream;
      const auto start = steady_clock::now();
      writer.write_symbols(stream);
      timings.write_symbols_after_shrink += steady_clock::now() - start;
      benchmark::DoNotOptimize(stream.view().size());
    }

    {
      IndexWriter writer{lss};
      std::ostringstream stream;
      prepare_writer_before_series(writer, stream);
      reset_stream(stream);
      const auto start = steady_clock::now();
      writer.write_series(ls_id, chunks, stream);
      timings.write_next_series_batch += steady_clock::now() - start;
      benchmark::DoNotOptimize(stream.view().size());
    }

    {
      IndexWriter writer{lss};
      std::ostringstream stream;
      prepare_writer_before_label_indices(writer, stream, ls_id, chunks);
      reset_stream(stream);
      const auto start = steady_clock::now();
      writer.write_label_indices(stream);
      timings.write_label_indices += steady_clock::now() - start;
      benchmark::DoNotOptimize(stream.view().size());
    }

    {
      IndexWriter writer{lss};
      std::ostringstream stream;
      prepare_writer_before_label_indices(writer, stream, ls_id, chunks);
      reset_stream(stream);
      writer.write_label_indices(stream);
      reset_stream(stream);
      const auto start = steady_clock::now();
      writer.write_postings(stream, kPostingsBatchSize);
      timings.write_next_postings_batch += steady_clock::now() - start;
      benchmark::DoNotOptimize(stream.view().size());
    }

    {
      IndexWriter writer{lss};
      std::ostringstream stream;
      prepare_writer_before_label_indices(writer, stream, ls_id, chunks);
      reset_stream(stream);
      writer.write_label_indices(stream);
      reset_stream(stream);
      const auto start = steady_clock::now();
      writer.write_label_indices_table(stream);
      timings.write_label_indices_table += steady_clock::now() - start;
      benchmark::DoNotOptimize(stream.view().size());
    }

    {
      IndexWriter writer{lss};
      std::ostringstream stream;
      prepare_writer_before_label_indices(writer, stream, ls_id, chunks);
      reset_stream(stream);
      writer.write_label_indices(stream);
      write_all_postings(writer, stream);
      reset_stream(stream);
      const auto start = steady_clock::now();
      writer.write_postings_table_offsets(stream);
      timings.write_postings_table_offsets += steady_clock::now() - start;
      benchmark::DoNotOptimize(stream.view().size());
    }

    {
      IndexWriter writer{lss};
      std::ostringstream stream;
      prepare_writer_before_label_indices(writer, stream, ls_id, chunks);
      reset_stream(stream);
      writer.write_label_indices(stream);
      write_all_postings(writer, stream);
      reset_stream(stream);
      writer.write_label_indices_table(stream);
      reset_stream(stream);
      writer.write_postings_table_offsets(stream);
      reset_stream(stream);
      const auto start = steady_clock::now();
      writer.write_toc(stream);
      timings.write_table_of_contents += steady_clock::now() - start;
      benchmark::DoNotOptimize(stream.view().size());
    }
  }
}

#define MIN_CALL_TIME(field)                                                                                                               \
  [](const auto&) {                                                                                                                        \
    return std::chrono::duration<double>(                                                                                                  \
               std::ranges::min_element(index_writer_call_timings, [](const auto& a, const auto& b) { return a.field < b.field; })->field) \
        .count();                                                                                                                          \
  }

BENCHMARK(IndexWriterEntrypointCalls)
    // We explicitly set Iterations(1) for the correct calculation benchmark times
    ->Iterations(1)
    ->ComputeStatistics("min_write_header", MIN_CALL_TIME(write_header))
    ->ComputeStatistics("min_write_symbols_no_shrink", MIN_CALL_TIME(write_symbols_no_shrink))
    ->ComputeStatistics("min_write_symbols_fixed_state", MIN_CALL_TIME(write_symbols_fixed_state))
    ->ComputeStatistics("min_write_symbols_after_shrink", MIN_CALL_TIME(write_symbols_after_shrink))
    ->ComputeStatistics("min_write_next_series_batch", MIN_CALL_TIME(write_next_series_batch))
    ->ComputeStatistics("min_write_label_indices", MIN_CALL_TIME(write_label_indices))
    ->ComputeStatistics("min_write_next_postings_batch", MIN_CALL_TIME(write_next_postings_batch))
    ->ComputeStatistics("min_write_label_indices_table", MIN_CALL_TIME(write_label_indices_table))
    ->ComputeStatistics("min_write_postings_table_offsets", MIN_CALL_TIME(write_postings_table_offsets))
    ->ComputeStatistics("min_write_table_of_contents", MIN_CALL_TIME(write_table_of_contents));

#undef MIN_CALL_TIME

}  // namespace
