#include <benchmark/benchmark.h>

#include <numeric>
#include <random>

#include "bare_bones/stream_v_byte.h"
#include "bare_bones/encoding.h"
#include "primitives/sample.h"
#include "series_data/data_storage.h"
#include "series_data/encoder.h"

namespace {
constexpr PromPP::Primitives::Sample::timestamp_type ts_min = 1698395400012;

template <class Codec>
using DataSequence = BareBones::StreamVByte::CompactSequence<Codec>;
template <template <class> class Encoding, class Codec>
using EncodingDataSequence = BareBones::EncodedSequence<Encoding<DataSequence<Codec>>>;

struct PROMPP_ATTRIBUTE_PACKED sample_with_lsid {
  PromPP::Primitives::Sample::value_type sample_value;
  uint32_t sample_ts;
  uint32_t labelset_id;
};

const BareBones::Vector<sample_with_lsid>& get_samples_for_benchmark() {
  constexpr auto get_file_name = [] -> std::string {
    if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
      return context->operator[]("sde_file");
    }

    return {};
  };

  static BareBones::Vector<sample_with_lsid> samples_from_file;
  if (samples_from_file.empty()) [[unlikely]] {
    std::ifstream istrm(get_file_name(), std::ios::binary);
    istrm >> samples_from_file;
  }

  return samples_from_file;
}

double min_value(const std::vector<double>& v) noexcept {
  return *std::ranges::min_element(v);
}

template <template <class> class Encoding, class Codec>
void BenchmarkLsIDEncoding(benchmark::State& state) {
  const auto& samples = get_samples_for_benchmark();

  series_data::DataStorage storage;
  series_data::Encoder encoder{storage};

  for (const auto& sample : samples) {
    encoder.encode(sample.labelset_id, ts_min + static_cast<PromPP::Primitives::Sample::timestamp_type>(sample.sample_ts), sample.sample_value);
  }

  constexpr size_t kValuesCount = 1'000'000;
  std::vector<uint32_t> numbers(kValuesCount);
  std::iota(numbers.begin(), numbers.end(), 0);

  const size_t ids_size = 9 * kValuesCount / 10;

  std::mt19937 gen(42);

  std::ranges::shuffle(numbers, gen);
  std::vector<uint32_t> ids(numbers.begin(), numbers.begin() + ids_size);
  std::ranges::sort(ids);

  auto it = std::stable_partition(ids.begin(), ids.end(), [&](const auto ls_id) {
    return series_data::is_gorilla_based_encoder(storage.open_chunks[ls_id].encoding_state.encoding_type);
  });
  ids.erase(it, ids.end());

  for ([[maybe_unused]] auto _ : state) {
    EncodingDataSequence<Encoding, Codec> sequence;
    for (const auto id : ids) {
      sequence.push_back(id);
    }
    benchmark::DoNotOptimize(sequence);
  }

  EncodingDataSequence<Encoding, Codec> sequence;
  for (const auto id : ids) {
    sequence.push_back(id);
  }

  state.counters["Uncompressed"] =
      benchmark::Counter(static_cast<double>(ids.size() * sizeof(uint32_t)), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);

  state.counters["Compressed"] = benchmark::Counter(static_cast<double>(sequence.allocated_memory()), benchmark::Counter::kDefaults,
                                                    benchmark::Counter::OneK::kIs1024);
}

BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaRLE, BareBones::StreamVByte::Codec1234>)->Unit(benchmark::kMillisecond)->
                                                                                                    ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaRLE, BareBones::StreamVByte::Codec1234Mostly1>)->Unit(benchmark::kMillisecond)->
                                                                                                       ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaRLE, BareBones::StreamVByte::Codec0124>)->Unit(benchmark::kMillisecond)->
                                                                                                    ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaRLE, BareBones::StreamVByte::Codec0124Frequent0>)->Unit(benchmark::kMillisecond)->
                                                                                                         ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaRLE, BareBones::StreamVByte::Codec1238>)->Unit(benchmark::kMillisecond)->
                                                                                                    ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaRLE, BareBones::StreamVByte::Codec1238Mostly1>)->Unit(benchmark::kMillisecond)->
                                                                                                       ComputeStatistics("min", min_value);

BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaZigZagRLE, BareBones::StreamVByte::Codec1234>)->Unit(benchmark::kMillisecond)->
                                                                                                ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaZigZagRLE, BareBones::StreamVByte::Codec1234Mostly1>)->Unit(benchmark::kMillisecond)->
                                                                                                       ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaZigZagRLE, BareBones::StreamVByte::Codec0124>)->Unit(benchmark::kMillisecond)->
                                                                                                ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaZigZagRLE, BareBones::StreamVByte::Codec0124Frequent0>)->Unit(benchmark::kMillisecond)->
                                                                                                         ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaZigZagRLE, BareBones::StreamVByte::Codec1238>)->Unit(benchmark::kMillisecond)->
                                                                                                ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaZigZagRLE, BareBones::StreamVByte::Codec1238Mostly1>)->Unit(benchmark::kMillisecond)->
                                                                                                       ComputeStatistics("min", min_value);

BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaDeltaZigZagRLE, BareBones::StreamVByte::Codec1234>)->Unit(benchmark::kMillisecond)->
                                                                                                ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaDeltaZigZagRLE, BareBones::StreamVByte::Codec1234Mostly1>)->Unit(benchmark::kMillisecond)->
                                                                                                       ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaDeltaZigZagRLE, BareBones::StreamVByte::Codec0124>)->Unit(benchmark::kMillisecond)->
                                                                                                ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaDeltaZigZagRLE, BareBones::StreamVByte::Codec0124Frequent0>)->Unit(benchmark::kMillisecond)->
                                                                                                         ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaDeltaZigZagRLE, BareBones::StreamVByte::Codec1238>)->Unit(benchmark::kMillisecond)->
                                                                                                ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaDeltaZigZagRLE, BareBones::StreamVByte::Codec1238Mostly1>)->Unit(benchmark::kMillisecond)->
                                                                                                       ComputeStatistics("min", min_value);

BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::Delta, BareBones::StreamVByte::Codec1234>)->Unit(benchmark::kMillisecond)->
                                                                                                 ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::Delta, BareBones::StreamVByte::Codec1234Mostly1>)->Unit(benchmark::kMillisecond)->
                                                                                                        ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::Delta, BareBones::StreamVByte::Codec0124>)->Unit(benchmark::kMillisecond)->
                                                                                                 ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::Delta, BareBones::StreamVByte::Codec0124Frequent0>)->Unit(benchmark::kMillisecond)->
                                                                                                         ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::Delta, BareBones::StreamVByte::Codec1238>)->Unit(benchmark::kMillisecond)->
                                                                                                 ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::Delta, BareBones::StreamVByte::Codec1238Mostly1>)->Unit(benchmark::kMillisecond)->
                                                                                                        ComputeStatistics("min", min_value);

BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaDeltaZigZag, BareBones::StreamVByte::Codec1234>)->Unit(benchmark::kMillisecond)->
                                                                                                ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaDeltaZigZag, BareBones::StreamVByte::Codec1234Mostly1>)->Unit(benchmark::kMillisecond)->
                                                                                                       ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaDeltaZigZag, BareBones::StreamVByte::Codec0124>)->Unit(benchmark::kMillisecond)->
                                                                                                ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaDeltaZigZag, BareBones::StreamVByte::Codec0124Frequent0>)->Unit(benchmark::kMillisecond)->
                                                                                                         ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaDeltaZigZag, BareBones::StreamVByte::Codec1238>)->Unit(benchmark::kMillisecond)->
                                                                                                ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaDeltaZigZag, BareBones::StreamVByte::Codec1238Mostly1>)->Unit(benchmark::kMillisecond)->
                                                                                                       ComputeStatistics("min", min_value);

template <template <class> class Encoding, class Codec>
void BenchmarkLengthEncoding(benchmark::State& state) {
  const auto& samples = get_samples_for_benchmark();

  series_data::DataStorage storage;
  series_data::Encoder encoder{storage};

  for (const auto& sample : samples) {
    encoder.encode(sample.labelset_id, ts_min + static_cast<PromPP::Primitives::Sample::timestamp_type>(sample.sample_ts), sample.sample_value);
  }

  constexpr size_t kValuesCount = 1'000'000;
  std::vector<uint32_t> numbers(kValuesCount);
  std::iota(numbers.begin(), numbers.end(), 0);

  const size_t ids_size = 9 * kValuesCount / 10;

  std::mt19937 gen(42);

  std::ranges::shuffle(numbers, gen);
  std::vector<uint32_t> ids(numbers.begin(), numbers.begin() + ids_size);
  std::ranges::sort(ids);

  auto it = std::stable_partition(ids.begin(), ids.end(), [&](const auto ls_id) {
    return series_data::is_gorilla_based_encoder(storage.open_chunks[ls_id].encoding_state.encoding_type);
  });
  ids.erase(it, ids.end());

  std::vector<uint32_t> lengths;
  lengths.resize(ids.size());

  size_t length_in_bytes = 0;

  for (auto id : ids) {
    switch (storage.open_chunks[id].encoding_state.encoding_type) {
      case series_data::EncodingType::kAscInteger: {
        lengths.push_back(
            storage.get_asc_integer_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).size_in_bits());
        length_in_bytes += storage.get_asc_integer_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).
                                   size_in_bytes();
        break;
      }
      case series_data::EncodingType::kAscIntegerThenValuesGorilla: {
        lengths.push_back(
            storage.get_asc_integer_then_values_gorilla_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).
                    size_in_bits());
        length_in_bytes += storage.get_asc_integer_then_values_gorilla_stream<series_data::chunk::DataChunk::Type::kOpen>(
                                       storage.open_chunks[id].encoder.external_index).
                                   size_in_bytes();
        break;
      }
      case series_data::EncodingType::kValuesGorilla: {
        lengths.push_back(
            storage.get_values_gorilla_stream<series_data::chunk::DataChunk::Type::kOpen>(
                storage.open_chunks[id].encoder.external_index).size_in_bits());
        length_in_bytes += storage.get_values_gorilla_stream<series_data::chunk::DataChunk::Type::kOpen>(
            storage.open_chunks[id].encoder.external_index).size_in_bytes();
        break;
      }
      case series_data::EncodingType::kGorilla: {
        lengths.push_back(
            storage.get_gorilla_encoder_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).size_in_bits());
        length_in_bytes += storage.get_gorilla_encoder_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).
                                   size_in_bytes();
        break;
      }
      default: { break; }
    }
  }

  for ([[maybe_unused]] auto _ : state) {
    EncodingDataSequence<Encoding, Codec> sequence;
    for (const auto len : lengths) {
      sequence.push_back(len);
    }
    benchmark::DoNotOptimize(sequence);
  }

  EncodingDataSequence<Encoding, Codec> sequence;
  for (const auto len : lengths) {
    sequence.push_back(len);
  }

  state.counters["Uncompressed"] =
      benchmark::Counter(static_cast<double>(ids.size() * sizeof(uint32_t)), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);

  state.counters["Compressed"] = benchmark::Counter(static_cast<double>(sequence.allocated_memory()), benchmark::Counter::kDefaults,
                                                    benchmark::Counter::OneK::kIs1024);

  state.counters["BitSeq"] = benchmark::Counter(static_cast<double>(length_in_bytes), benchmark::Counter::kDefaults,
                                                benchmark::Counter::OneK::kIs1024);
}

BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaRLE, BareBones::StreamVByte::Codec1234>)->Unit(benchmark::kMillisecond)->
                                                                                                      ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaRLE, BareBones::StreamVByte::Codec1234Mostly1>)->Unit(benchmark::kMillisecond)->
                                                                                                        ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaRLE, BareBones::StreamVByte::Codec0124>)->Unit(benchmark::kMillisecond)->
                                                                                                      ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaRLE, BareBones::StreamVByte::Codec0124Frequent0>)->Unit(benchmark::kMillisecond)->
                                                                                                          ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaRLE, BareBones::StreamVByte::Codec1238>)->Unit(benchmark::kMillisecond)->
                                                                                                      ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaRLE, BareBones::StreamVByte::Codec1238Mostly1>)->Unit(benchmark::kMillisecond)->
                                                                                                        ComputeStatistics("min", min_value);

BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaZigZagRLE, BareBones::StreamVByte::Codec1234>)->Unit(benchmark::kMillisecond)->
                                                                                                 ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaZigZagRLE, BareBones::StreamVByte::Codec1234Mostly1>)->Unit(benchmark::kMillisecond)->
                                                                                                        ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaZigZagRLE, BareBones::StreamVByte::Codec0124>)->Unit(benchmark::kMillisecond)->
                                                                                                 ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaZigZagRLE, BareBones::StreamVByte::Codec0124Frequent0>)->Unit(benchmark::kMillisecond)->
                                                                                                          ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaZigZagRLE, BareBones::StreamVByte::Codec1238>)->Unit(benchmark::kMillisecond)->
                                                                                                 ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaZigZagRLE, BareBones::StreamVByte::Codec1238Mostly1>)->Unit(benchmark::kMillisecond)->
                                                                                                        ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaDeltaZigZagRLE, BareBones::StreamVByte::Codec1234>)->Unit(benchmark::kMillisecond)->
                                                                                                ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaDeltaZigZagRLE, BareBones::StreamVByte::Codec1234Mostly1>)->Unit(benchmark::kMillisecond)->
                                                                                                       ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaDeltaZigZagRLE, BareBones::StreamVByte::Codec0124>)->Unit(benchmark::kMillisecond)->
                                                                                                ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaDeltaZigZagRLE, BareBones::StreamVByte::Codec0124Frequent0>)->Unit(benchmark::kMillisecond)->
                                                                                                         ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaDeltaZigZagRLE, BareBones::StreamVByte::Codec1238>)->Unit(benchmark::kMillisecond)->
                                                                                                ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaDeltaZigZagRLE, BareBones::StreamVByte::Codec1238Mostly1>)->Unit(benchmark::kMillisecond)->
                                                                                                       ComputeStatistics("min", min_value);

BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::Delta, BareBones::StreamVByte::Codec1234>)->Unit(benchmark::kMillisecond)->
                                                                                                   ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::Delta, BareBones::StreamVByte::Codec1234Mostly1>)->Unit(benchmark::kMillisecond)->
                                                                                                         ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::Delta, BareBones::StreamVByte::Codec0124>)->Unit(benchmark::kMillisecond)->
                                                                                                   ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::Delta, BareBones::StreamVByte::Codec0124Frequent0>)->Unit(benchmark::kMillisecond)->
                                                                                                          ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::Delta, BareBones::StreamVByte::Codec1238>)->Unit(benchmark::kMillisecond)->
                                                                                                   ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::Delta, BareBones::StreamVByte::Codec1238Mostly1>)->Unit(benchmark::kMillisecond)->
                                                                                                         ComputeStatistics("min", min_value);

BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaDeltaZigZag, BareBones::StreamVByte::Codec1234>)->Unit(benchmark::kMillisecond)->
                                                                                                 ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaDeltaZigZag, BareBones::StreamVByte::Codec1234Mostly1>)->Unit(benchmark::kMillisecond)->
                                                                                                        ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaDeltaZigZag, BareBones::StreamVByte::Codec0124>)->Unit(benchmark::kMillisecond)->
                                                                                                 ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaDeltaZigZag, BareBones::StreamVByte::Codec0124Frequent0>)->Unit(benchmark::kMillisecond)->
                                                                                                         ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaDeltaZigZag, BareBones::StreamVByte::Codec1238>)->Unit(benchmark::kMillisecond)->
                                                                                                 ComputeStatistics("min", min_value);
BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaDeltaZigZag, BareBones::StreamVByte::Codec1238Mostly1>)->Unit(benchmark::kMillisecond)->
                                                                                                        ComputeStatistics("min", min_value);
} // namespace