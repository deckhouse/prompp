#include <benchmark/benchmark.h>

#include <numeric>
#include <random>

#include "bare_bones/encoding.h"
#include "bare_bones/lz4_stream.h"
#include "bare_bones/stream_v_byte.h"
#include "primitives/sample.h"
#include "series_data/data_storage.h"
#include "series_data/encoder.h"
#include "series_data/encoder/zig_zag_timestamp_gorilla.h"

namespace {
constexpr PromPP::Primitives::Sample::timestamp_type ts_min = 1698395400012;

template <class Codec>
using DataSequence = BareBones::StreamVByte::Sequence<Codec>;
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

  const size_t kValuesCount = storage.open_chunks.size();
  std::vector<uint32_t> numbers(kValuesCount);
  std::iota(numbers.begin(), numbers.end(), 0);

  const size_t ids_size = 9 * kValuesCount / 10;

  std::mt19937 gen(42);

  std::ranges::shuffle(numbers, gen);
  std::vector<uint32_t> ids(numbers.begin(), numbers.begin() + ids_size);
  std::ranges::sort(ids);

  auto it = std::stable_partition(
      ids.begin(), ids.end(), [&](const auto ls_id) { return series_data::is_gorilla_based_encoder(storage.open_chunks[ls_id].encoding_state.encoding_type); });
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

  state.counters["Compressed"] =
      benchmark::Counter(static_cast<double>(sequence.allocated_memory()), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);
}

BENCHMARK(BenchmarkLsIDEncoding<BareBones::Encoding::DeltaDeltaZigZag, BareBones::StreamVByte::Codec0124Frequent0>)
    ->Unit(benchmark::kMillisecond)
    ->ComputeStatistics("min", min_value);

void BenchmarkLsIDTimestampEncoding(benchmark::State& state) {
  const auto& samples = get_samples_for_benchmark();

  series_data::DataStorage storage;
  series_data::Encoder encoder{storage};

  for (const auto& sample : samples) {
    encoder.encode(sample.labelset_id, ts_min + static_cast<PromPP::Primitives::Sample::timestamp_type>(sample.sample_ts), sample.sample_value);
  }

  const size_t kValuesCount = storage.open_chunks.size();
  std::vector<uint32_t> numbers(kValuesCount);
  std::iota(numbers.begin(), numbers.end(), 0);

  const size_t ids_size = 9 * kValuesCount / 10;

  std::mt19937 gen(42);

  std::ranges::shuffle(numbers, gen);
  std::vector<uint32_t> ids(numbers.begin(), numbers.begin() + ids_size);
  std::ranges::sort(ids);

  auto it = std::stable_partition(
      ids.begin(), ids.end(), [&](const auto ls_id) { return series_data::is_gorilla_based_encoder(storage.open_chunks[ls_id].encoding_state.encoding_type); });
  ids.erase(it, ids.end());

  for ([[maybe_unused]] auto _ : state) {
    series_data::encoder::ZigZagTimestampEncoder<int32_t> encoder;
    BareBones::BitSequence sequence;

    uint8_t cnt = 0;
    for (const auto id : ids) {
      if (cnt == 0) {
        encoder.encode(id, sequence);
        ++cnt;
      } else if (cnt == 1) {
        encoder.encode_delta(id, sequence);
        ++cnt;
      } else {
        encoder.encode_delta_of_delta(static_cast<int64_t>(id), sequence);
      }
    }

    benchmark::DoNotOptimize(sequence);

    state.counters["Uncompressed"] =
        benchmark::Counter(static_cast<double>(ids.size() * sizeof(uint32_t)), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);

    state.counters["Compressed"] =
        benchmark::Counter(static_cast<double>(sequence.allocated_memory()), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);
  }
}

BENCHMARK(BenchmarkLsIDTimestampEncoding)->Unit(benchmark::kMillisecond)->Iterations(1)->ComputeStatistics("min", min_value);

template <template <class> class Encoding, class Codec>
void BenchmarkLengthEncoding(benchmark::State& state) {
  const auto& samples = get_samples_for_benchmark();

  series_data::DataStorage storage;
  series_data::Encoder encoder{storage};

  for (const auto& sample : samples) {
    const size_t i = state.range(0);
    int64_t min_ts = 5 * 60 * 1000 * i;
    int64_t max_ts = 5 * 60 * 1000 * (i + 1);
    if (min_ts <= sample.sample_ts && sample.sample_ts < max_ts) {
      encoder.encode(sample.labelset_id, ts_min + static_cast<PromPP::Primitives::Sample::timestamp_type>(sample.sample_ts), sample.sample_value);
    }
  }

  const size_t kValuesCount = storage.open_chunks.size();
  std::vector<uint32_t> numbers(kValuesCount);
  std::iota(numbers.begin(), numbers.end(), 0);

  const size_t ids_size = 9 * kValuesCount / 10;

  std::mt19937 gen(42);

  std::ranges::shuffle(numbers, gen);
  std::vector<uint32_t> ids(numbers.begin(), numbers.begin() + ids_size);
  std::ranges::sort(ids);

  auto it = std::stable_partition(
      ids.begin(), ids.end(), [&](const auto ls_id) { return series_data::is_gorilla_based_encoder(storage.open_chunks[ls_id].encoding_state.encoding_type); });
  ids.erase(it, ids.end());

  std::vector<uint32_t> lengths;
  lengths.reserve(ids.size());

  size_t length_in_bytes = 0;

  for (auto id : ids) {
    switch (storage.open_chunks[id].encoding_state.encoding_type) {
      case series_data::EncodingType::kAscInteger: {
        lengths.push_back(
            storage.get_asc_integer_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).size_in_bits());
        length_in_bytes +=
            storage.get_asc_integer_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).size_in_bytes();
        break;
      }
      case series_data::EncodingType::kAscIntegerThenValuesGorilla: {
        lengths.push_back(
            storage.get_asc_integer_then_values_gorilla_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index)
                .size_in_bits());
        length_in_bytes +=
            storage.get_asc_integer_then_values_gorilla_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index)
                .size_in_bytes();
        break;
      }
      case series_data::EncodingType::kValuesGorilla: {
        lengths.push_back(
            storage.get_values_gorilla_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).size_in_bits());
        length_in_bytes +=
            storage.get_values_gorilla_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).size_in_bytes();
        break;
      }
      case series_data::EncodingType::kGorilla: {
        lengths.push_back(
            storage.get_gorilla_encoder_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).size_in_bits());
        length_in_bytes +=
            storage.get_gorilla_encoder_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).size_in_bytes();
        break;
      }
      default: {
        break;
      }
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
      benchmark::Counter(static_cast<double>(lengths.size() * sizeof(uint32_t)), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);

  state.counters["Compressed"] =
      benchmark::Counter(static_cast<double>(sequence.allocated_memory()), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);

  state.counters["BitSeq"] = benchmark::Counter(static_cast<double>(length_in_bytes), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);
}

BENCHMARK(BenchmarkLengthEncoding<BareBones::Encoding::DeltaDeltaZigZag, BareBones::StreamVByte::Codec0124Frequent0>)
    ->DenseRange(0, 11, 1)
    ->Unit(benchmark::kMillisecond)
    ->ComputeStatistics("min", min_value);

void BenchmarkLengthTimestampEncoding(benchmark::State& state) {
  const auto& samples = get_samples_for_benchmark();

  series_data::DataStorage storage;
  series_data::Encoder encoder{storage};

  for (const auto& sample : samples) {
    const size_t i = state.range(0);
    int64_t min_ts = 5 * 60 * 1000 * i;
    int64_t max_ts = 5 * 60 * 1000 * (i + 1);
    if (min_ts <= sample.sample_ts && sample.sample_ts < max_ts) {
      encoder.encode(sample.labelset_id, ts_min + static_cast<PromPP::Primitives::Sample::timestamp_type>(sample.sample_ts), sample.sample_value);
    }
  }

  const size_t kValuesCount = storage.open_chunks.size();
  std::vector<uint32_t> numbers(kValuesCount);
  std::iota(numbers.begin(), numbers.end(), 0);

  const size_t ids_size = 9 * kValuesCount / 10;

  std::mt19937 gen(42);

  std::ranges::shuffle(numbers, gen);
  std::vector<uint32_t> ids(numbers.begin(), numbers.begin() + ids_size);
  std::ranges::sort(ids);

  auto it = std::stable_partition(
      ids.begin(), ids.end(), [&](const auto ls_id) { return series_data::is_gorilla_based_encoder(storage.open_chunks[ls_id].encoding_state.encoding_type); });
  ids.erase(it, ids.end());

  std::vector<uint32_t> lengths;
  lengths.reserve(ids.size());

  size_t length_in_bytes = 0;

  for (auto id : ids) {
    switch (storage.open_chunks[id].encoding_state.encoding_type) {
      case series_data::EncodingType::kAscInteger: {
        lengths.push_back(
            storage.get_asc_integer_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).size_in_bits());
        length_in_bytes +=
            storage.get_asc_integer_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).size_in_bytes();
        break;
      }
      case series_data::EncodingType::kAscIntegerThenValuesGorilla: {
        lengths.push_back(
            storage.get_asc_integer_then_values_gorilla_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index)
                .size_in_bits());
        length_in_bytes +=
            storage.get_asc_integer_then_values_gorilla_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index)
                .size_in_bytes();
        break;
      }
      case series_data::EncodingType::kValuesGorilla: {
        lengths.push_back(
            storage.get_values_gorilla_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).size_in_bits());
        length_in_bytes +=
            storage.get_values_gorilla_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).size_in_bytes();
        break;
      }
      case series_data::EncodingType::kGorilla: {
        lengths.push_back(
            storage.get_gorilla_encoder_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).size_in_bits());
        length_in_bytes +=
            storage.get_gorilla_encoder_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).size_in_bytes();
        break;
      }
      default: {
        break;
      }
    }
  }

  for ([[maybe_unused]] auto _ : state) {
    series_data::encoder::ZigZagTimestampEncoder<int32_t> encoder;
    BareBones::BitSequence sequence;

    uint8_t cnt = 0;
    for (const auto len : lengths) {
      if (cnt == 0) {
        encoder.encode(len, sequence);
        ++cnt;
      } else if (cnt == 1) {
        encoder.encode_delta(len, sequence);
        ++cnt;
      } else {
        encoder.encode_delta_of_delta(static_cast<int64_t>(len), sequence);
      }
    }

    benchmark::DoNotOptimize(sequence);

    state.counters["Uncompressed"] =
        benchmark::Counter(static_cast<double>(lengths.size() * sizeof(uint32_t)), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);

    state.counters["Compressed"] =
        benchmark::Counter(static_cast<double>(sequence.allocated_memory()), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);
  }
}

BENCHMARK(BenchmarkLengthTimestampEncoding)->DenseRange(0, 11, 1)->Unit(benchmark::kMillisecond)->ComputeStatistics("min", min_value);

template <template <class> class Encoding, class Codec>
void BenchmarkLZ4Encoding(benchmark::State& state) {
  for ([[maybe_unused]] auto _ : state) {
    const auto& samples = get_samples_for_benchmark();

    series_data::DataStorage storage;
    series_data::Encoder encoder{storage};

    // std::ostringstream stream;
    std::ostringstream lz4stream;  //&stream};

    size_t length_in_bytes = 0;

    size_t i = 0;
    for (const auto& sample : samples) {
      int64_t max_ts = 5 * 60 * 1000 * (i + 1);
      encoder.encode(sample.labelset_id, ts_min + static_cast<PromPP::Primitives::Sample::timestamp_type>(sample.sample_ts), sample.sample_value);
      if (sample.sample_ts > max_ts) {
        ++i;

        std::ostringstream bitstream;

        const size_t kValuesCount = storage.open_chunks.size();
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
        lengths.reserve(ids.size());

        for (auto id : ids) {
          switch (storage.open_chunks[id].encoding_state.encoding_type) {
            case series_data::EncodingType::kAscInteger: {
              lengths.push_back(
                  storage.get_asc_integer_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).size_in_bits());
              length_in_bytes +=
                  storage.get_asc_integer_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).size_in_bytes();
              for (auto byte :
                   storage.get_asc_integer_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).bytes()) {
                bitstream << byte;
              }
              storage.variant_encoders[storage.open_chunks[id].encoder.external_index].asc_integer.stream().rewind();
              break;
            }
            case series_data::EncodingType::kAscIntegerThenValuesGorilla: {
              lengths.push_back(
                  storage.get_asc_integer_then_values_gorilla_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index)
                      .size_in_bits());
              length_in_bytes +=
                  storage.get_asc_integer_then_values_gorilla_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index)
                      .size_in_bytes();
              for (auto byte :
                   storage
                       .get_asc_integer_then_values_gorilla_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index)
                       .bytes()) {
                bitstream << byte;
              }
              storage.variant_encoders[storage.open_chunks[id].encoder.external_index].asc_integer_then_values_gorilla.stream().rewind();
              break;
            }
            case series_data::EncodingType::kValuesGorilla: {
              lengths.push_back(
                  storage.get_values_gorilla_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).size_in_bits());
              length_in_bytes +=
                  storage.get_values_gorilla_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).size_in_bytes();
              for (auto byte :
                   storage.get_values_gorilla_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).bytes()) {
                bitstream << byte;
              }
              storage.variant_encoders[storage.open_chunks[id].encoder.external_index].values_gorilla.stream().rewind();
              break;
            }
            case series_data::EncodingType::kGorilla: {
              // lengths.push_back(storage.get_gorilla_encoder_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index)
              //                       .size_in_bits());
              // length_in_bytes +=
              // storage.get_gorilla_encoder_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index)
              //                        .size_in_bytes();
              // for (auto byte :
              //      storage.get_gorilla_encoder_stream<series_data::chunk::DataChunk::Type::kOpen>(storage.open_chunks[id].encoder.external_index).bytes())
              //      {
              //   bitstream << byte;
              // }
              // storage.gorilla_encoders[storage.open_chunks[id].encoder.external_index].stream().stream.rewind();
              // break;
            }
            default: {
              break;
            }
          }
        }

        EncodingDataSequence<Encoding, Codec> length_sequence;
        for (const auto len : lengths) {
          length_sequence.push_back(len);
        }

        EncodingDataSequence<Encoding, Codec> id_sequence;
        for (const auto id : ids) {
          id_sequence.push_back(id);
        }

        bitstream << std::flush;

        lz4stream << id_sequence << length_sequence << bitstream.str() << std::flush;
      }
    }
    state.counters["LZ4"] = benchmark::Counter(static_cast<double>(lz4stream.str().size()), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);
    state.counters["BitSeq"] = benchmark::Counter(static_cast<double>(length_in_bytes), benchmark::Counter::kDefaults, benchmark::Counter::OneK::kIs1024);
  }
}

BENCHMARK(BenchmarkLZ4Encoding<BareBones::Encoding::DeltaDeltaZigZag, BareBones::StreamVByte::Codec0124Frequent0>)
    ->Unit(benchmark::kMillisecond)
    ->ComputeStatistics("min", min_value);
}  // namespace