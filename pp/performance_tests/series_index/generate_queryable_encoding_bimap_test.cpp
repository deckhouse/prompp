#include "generate_queryable_encoding_bimap_test.h"

#include <chrono>

#include "performance_tests/dummy_wal.h"
#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"
#include "wal/wal.h"

namespace performance_tests::series_index {

using TrieIndex = ::series_index::TrieIndex<::series_index::trie::CedarTrie, ::series_index::trie::CedarMatchesList>;
using QueryableEncodingBimap =
    ::series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, BareBones::Vector, TrieIndex>;

void GenerateQueryableEncodingBimap::execute([[maybe_unused]] const Config& config, [[maybe_unused]] Metrics& metrics) const {
  DummyWal::Timeseries tmsr;
  DummyWal dummy_wal(input_file_full_name(config));

  QueryableEncodingBimap lss;
  //lss.reserve(1200000);

  uint32_t find_or_emplace_call_count = 0;
  uint32_t max_ls_id = std::numeric_limits<uint32_t>::min();
  std::chrono::nanoseconds find_or_emplace_time{};
  std::chrono::nanoseconds emplace_time{};
  while (dummy_wal.read_next_segment()) {
    while (dummy_wal.read_next(tmsr)) {
      auto start_tm = std::chrono::steady_clock::now();
      const auto ls_id = lss.find_or_emplace(tmsr.label_set());
      const auto elapsed_time = std::chrono::steady_clock::now() - start_tm;
      find_or_emplace_time += elapsed_time;
      ++find_or_emplace_call_count;
      if (ls_id > max_ls_id) {
        max_ls_id = ls_id;
        emplace_time += elapsed_time;
      }
      BareBones::compiler::do_not_optimize(start_tm);
      BareBones::compiler::do_not_optimize(find_or_emplace_call_count);
    }
  }

  std::vector<uint32_t> gg;
  const auto start_tm = std::chrono::steady_clock::now();
  lss.sort_series_ids(gg);

  std::cout << "lss_allocated_memory_kb: " << lss.allocated_memory() / 1024 << std::endl;
  std::cout << "build_sort_index_time_nanoseconds: " << (std::chrono::steady_clock::now() - start_tm).count() << std::endl;
  std::cout << "find_or_emplace_time_nanoseconds: " << find_or_emplace_time.count() / find_or_emplace_call_count << std::endl;
  std::cout << "emplace_time_nanoseconds: " << emplace_time.count() / (max_ls_id + 1) << std::endl;
}

}  // namespace performance_tests::series_index