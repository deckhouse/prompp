#include "chunk_recoder_test.h"

#include <chrono>
#include <iostream>

#include "bare_bones/gorilla.h"
#include "head/chunk_recoder.h"
#include "performance_tests/dummy_wal.h"
#include "primitives/snug_composites.h"
#include "series_data/encoder.h"
#include "series_data/outdated_sample_encoder.h"

namespace performance_tests {

using BareBones::Encoding::Gorilla::StreamEncoder;
using BareBones::Encoding::Gorilla::TimestampDecoder;
using BareBones::Encoding::Gorilla::TimestampEncoder;
using BareBones::Encoding::Gorilla::ValuesEncoder;
using PromPP::Primitives::TimeInterval;
using series_data::Decoder;
using series_data::encoder::Sample;
using series_data::encoder::SampleList;

void ChunkRecoder::execute(const Config& config, [[maybe_unused]] Metrics& metrics) const {
  DummyWal::Timeseries tmsr;
  DummyWal dummy_wal(input_file_full_name(config));

  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<BareBones::Vector> label_set_bitmap;
  series_data::DataStorage storage;
  series_data::Encoder encoder{storage};
  TimeInterval time_interval{.min = TimeInterval::kMax, .max = TimeInterval::kMin};

  while (dummy_wal.read_next_segment()) {
    while (dummy_wal.read_next(tmsr)) {
      const auto ls_id = label_set_bitmap.find_or_emplace(tmsr.label_set());
      auto& sample = tmsr.samples()[0];
      encoder.encode(ls_id, sample.timestamp(), sample.value());

      time_interval.min = std::min(time_interval.min, sample.timestamp());
      time_interval.max = std::max(time_interval.max, sample.timestamp());
    }
  }

  const std::ranges::iota_view<uint32_t, uint32_t> ls_id_set(0, label_set_bitmap.size() - 1);
  head::ChunkRecoder recoder(ls_id_set.begin(), ls_id_set.end(), &storage, time_interval);

  struct {
    TimeInterval interval;
    uint32_t series_id{};
    uint8_t samples_count{};
    BareBones::Vector<uint8_t> buffer;
  } chunk_data;

  const auto start_tm = std::chrono::steady_clock::now();
  while (recoder.has_more_data()) {
    recoder.recode_next_chunk(chunk_data);
  }

  std::cout << "recode time: " << (std::chrono::steady_clock::now() - start_tm).count() << std::endl;
}

}  // namespace performance_tests