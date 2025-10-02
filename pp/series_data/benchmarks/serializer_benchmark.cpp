#include <chrono>
#include <fstream>

#include <benchmark/benchmark.h>

#include <iostream>
#include <random>

#include "bare_bones/preprocess.h"
#include "series_data/encoder.h"
#include "series_data/querier/query.h"
#include "series_data/serialization/serialized_data.h"
#include "series_data/serialization/serializer.h"

namespace {

using BareBones::StreamVByte::CompactSequence;
using BareBones::StreamVByte::Sequence;

struct PROMPP_ATTRIBUTE_PACKED SeriesSample {
  uint32_t series_id;
  int64_t timestamp;
  double value;
};

const BareBones::Vector<SeriesSample>& get_samples_for_benchmark() {
  constexpr auto get_file_name = [] -> std::string {
    if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
      return context->operator[]("wal_file");
    }

    return {};
  };

  static BareBones::Vector<SeriesSample> samples_from_file;
  if (samples_from_file.empty()) [[likely]] {
    std::ifstream istrm(get_file_name(), std::ios::binary);
    istrm >> samples_from_file;
  }

  return samples_from_file;
}

void BenchmarkWalSerializer(benchmark::State& state) {
  const auto& samples = get_samples_for_benchmark();
  const double percent = state.range(0) / 100.0;
  const auto [min, max] = std::ranges::minmax_element(samples, [](auto a, auto b) { return a.timestamp < b.timestamp; });
  const auto min_ts = min->timestamp;
  const auto max_ts = max->timestamp;
  const auto delta_ts = max_ts - min_ts;

  series_data::DataStorage storage;
  series_data::Encoder encoder{storage};

  for (const auto& sample : samples) {
    if (sample.timestamp < min_ts + delta_ts * percent) {
      encoder.encode(sample.series_id, sample.timestamp, sample.value);
    }
  }

  series_data::querier::QueriedChunkList chunk_list;
  {
    std::vector<uint32_t> v(storage.open_chunks.size());
    std::iota(v.begin(), v.end(), 0);

    std::mt19937 g(42);
    std::ranges::shuffle(v, g);
    v.resize(v.size() / 10);

    chunk_list.reserve(v.size());
    for (uint32_t ls_id : v) {
      chunk_list.emplace_back(ls_id);
    }
  }

  for ([[maybe_unused]] auto _ : state) {
    series_data::serialization::Serializer serializer_{storage};
    BareBones::ShrinkedToFitOStringStream stream;

    serializer_.serialize(chunk_list, stream);
  }

  {
    series_data::serialization::Serializer serializer_{storage};
    BareBones::ShrinkedToFitOStringStream stream;

    serializer_.serialize(chunk_list, stream);
    state.counters["Stream Size"] = benchmark::Counter(stream.view().size(), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);
  }
}

void BenchmarkWalConstantSerializer(benchmark::State& state) {
  const auto& samples = get_samples_for_benchmark();
  const double percent = state.range(0) / 100.0;
  const auto [min, max] = std::ranges::minmax_element(samples, [](auto a, auto b) { return a.timestamp < b.timestamp; });
  const auto min_ts = min->timestamp;
  const auto max_ts = max->timestamp;
  const auto delta_ts = max_ts - min_ts;

  series_data::DataStorage storage;
  series_data::Encoder encoder{storage};

  for (const auto& sample : samples) {
    if (sample.timestamp <= min_ts + delta_ts * percent) {
      encoder.encode(sample.series_id, sample.timestamp, sample.series_id);
    }
  }

  series_data::querier::QueriedChunkList chunk_list;
  {
    std::vector<uint32_t> v(storage.open_chunks.size());
    std::iota(v.begin(), v.end(), 0);

    std::mt19937 g(42);
    std::ranges::shuffle(v, g);
    v.resize(v.size() / 10);

    chunk_list.reserve(v.size());
    for (uint32_t ls_id : v) {
      chunk_list.emplace_back(ls_id);
    }
  }

  for ([[maybe_unused]] auto _ : state) {
    series_data::serialization::Serializer serializer_{storage};
    BareBones::ShrinkedToFitOStringStream stream;

    serializer_.serialize(chunk_list, stream);
  }

  {
    series_data::serialization::Serializer serializer_{storage};
    BareBones::ShrinkedToFitOStringStream stream;

    serializer_.serialize(chunk_list, stream);
    state.counters["Stream Size"] = benchmark::Counter(stream.view().size(), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);
  }
}

void BenchmarkWalSerializedData(benchmark::State& state) {
  const auto& samples = get_samples_for_benchmark();
  const double percent = state.range(0) / 100.0;
  const auto [min, max] = std::ranges::minmax_element(samples, [](auto a, auto b) { return a.timestamp < b.timestamp; });
  const auto min_ts = min->timestamp;
  const auto max_ts = max->timestamp;
  const auto delta_ts = max_ts - min_ts;

  series_data::DataStorage storage;
  series_data::Encoder encoder{storage};

  for (const auto& sample : samples) {
    if (sample.timestamp < min_ts + delta_ts * percent) {
      encoder.encode(sample.series_id, sample.timestamp, sample.value);
    }
  }

  series_data::querier::QueriedChunkList chunk_list;
  {
    std::vector<uint32_t> v(storage.open_chunks.size());
    std::iota(v.begin(), v.end(), 0);

    std::mt19937 g(42);
    std::ranges::shuffle(v, g);
    v.resize(v.size() / 10);

    chunk_list.reserve(v.size());
    for (uint32_t ls_id : v) {
      chunk_list.emplace_back(ls_id);
    }
  }

  for ([[maybe_unused]] auto _ : state) {
    series_data::serialization::SerializedData serialized(storage, chunk_list);
    benchmark::DoNotOptimize(serialized);
  }

  {
    series_data::serialization::SerializedData serialized(storage, chunk_list);
    state.counters["Total Size"] = benchmark::Counter(serialized.allocated_memory(), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);
  }
}

void BenchmarkWalConstantSerializedData(benchmark::State& state) {
  const auto& samples = get_samples_for_benchmark();
  const double percent = state.range(0) / 100.0;
  const auto [min, max] = std::ranges::minmax_element(samples, [](auto a, auto b) { return a.timestamp < b.timestamp; });
  const auto min_ts = min->timestamp;
  const auto max_ts = max->timestamp;
  const auto delta_ts = max_ts - min_ts;

  series_data::DataStorage storage;
  series_data::Encoder encoder{storage};

  for (const auto& sample : samples) {
    if (sample.timestamp <= min_ts + delta_ts * percent) {
      encoder.encode(sample.series_id, sample.timestamp, sample.series_id);
    }
  }

  series_data::querier::QueriedChunkList chunk_list;
  {
    std::vector<uint32_t> v(storage.open_chunks.size());
    std::iota(v.begin(), v.end(), 0);

    std::mt19937 g(42);
    std::ranges::shuffle(v, g);
    v.resize(v.size() / 10);

    chunk_list.reserve(v.size());
    for (uint32_t ls_id : v) {
      chunk_list.emplace_back(ls_id);
    }
  }

  for ([[maybe_unused]] auto _ : state) {
    series_data::serialization::SerializedData serialized(storage, chunk_list);
    benchmark::DoNotOptimize(serialized);
  }

  {
    series_data::serialization::SerializedData serialized(storage, chunk_list);
    state.counters["Total Size"] = benchmark::Counter(serialized.allocated_memory(), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);
  }
}

BENCHMARK(BenchmarkWalSerializer)->Arg(25)->Arg(50)->Arg(75)->Arg(100);
BENCHMARK(BenchmarkWalSerializedData)->Arg(25)->Arg(50)->Arg(75)->Arg(100);
BENCHMARK(BenchmarkWalConstantSerializer)->Arg(25)->Arg(50)->Arg(75)->Arg(100);
BENCHMARK(BenchmarkWalConstantSerializedData)->Arg(25)->Arg(50)->Arg(75)->Arg(100);

}  // namespace
