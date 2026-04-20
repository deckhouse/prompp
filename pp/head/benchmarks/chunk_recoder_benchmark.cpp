#include <fstream>

#include <benchmark/benchmark.h>

#include "profiling/profiling.h"

#include "bare_bones/preprocess.h"
#include "head/chunk_recoder.h"
#include "primitives/sample.h"
#include "series_data/decoder.h"
#include "series_data/encoder.h"

namespace {

using DataStorage = series_data::DataStorage;
using PromPP::Primitives::TimeInterval;

const DataStorage& get_data_storage_for_benchmark() {
  struct PROMPP_ATTRIBUTE_PACKED sample_with_lsid {
    PromPP::Primitives::Sample::value_type sample_value;
    uint32_t sample_ts;
    uint32_t labelset_id;
  };

  constexpr auto get_file_name = [] -> std::string {
    if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
      return context->operator[]("sde_file");
    }

    return {};
  };

  static series_data::DataStorage storage;
  if (storage.open_chunks.empty()) [[unlikely]] {
    BareBones::Vector<sample_with_lsid> samples_from_file;
    {
      std::ifstream istrm(get_file_name(), std::ios::binary);
      istrm >> samples_from_file;
    }
    series_data::Encoder encoder{storage};
    for (const auto& sample : samples_from_file) {
      encoder.encode(sample.labelset_id, sample.sample_ts, sample.sample_value);
    }
  }

  return storage;
}

void BenchmarkChunkRecoder(benchmark::State& state) {
  ZoneScoped;
  const auto& storage = get_data_storage_for_benchmark();

  const auto time_interval = series_data::Decoder::get_time_interval(storage);
  const std::ranges::iota_view<uint32_t, uint32_t> ls_id_set(0, storage.open_chunks.size());
  head::ChunkRecoder recoder(head::ChunkRecoderIterator{ls_id_set.begin(), ls_id_set.end(), storage.open_chunks.size(), &storage, time_interval},
                             time_interval);

  struct {
    TimeInterval interval;
    uint32_t series_id{};
    uint8_t samples_count{};
  } chunk_data;

  for ([[maybe_unused]] auto _ : state) {
    while (recoder.has_more_data()) {
      recoder.recode_next_chunk(chunk_data);
    }
  }
}

BENCHMARK(BenchmarkChunkRecoder)->ComputeStatistics("min", [](const std::vector<double>& v) { return *std::ranges::min_element(v); });

}  // namespace
