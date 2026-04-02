#include "save_lss_to_wal_test.h"

#include "dummy_wal.h"
#include "log.h"
#include "primitives/snug_composites.h"

using namespace PromPP;  // NOLINT

void save_lss_to_wal::execute(const Config& config, Metrics& metrics) const {
  std::ofstream outfile(output_file_full_name(config), std::ios_base::binary);
  BareBones::LZ4Stream::ostream out(&outfile);
  if (!outfile.is_open()) {
    throw std::runtime_error("failed to open file '" + output_file_full_name(config) + "'");
  }

  DummyWal::Timeseries tmsr;
  DummyWal dummy_wal(input_file_full_name(config));
  Primitives::SnugComposites::LabelSet::EncodingBimap<BareBones::Vector> lss;

  auto checkpoint = lss.checkpoint();

  auto start = std::chrono::steady_clock::now();
  while (dummy_wal.read_next_segment()) {
    while (dummy_wal.read_next(tmsr)) {
      lss.find_or_emplace(tmsr.label_set());
      if (dummy_wal.cnt() % 1000000 == 0) {
        auto now = std::chrono::steady_clock::now();
        log() << "Processed: " << dummy_wal.cnt()
              << " Time per sample: " << (std::chrono::duration_cast<std::chrono::nanoseconds>(now - start).count() / dummy_wal.cnt()) << " ns" << std::endl;
        log() << "Number of label name symbols: " << lss.data_view().keys().size() << std::endl;
        log() << "Number of label name sets: " << lss.data_view().label_name_sets().size() << std::endl;
        log() << "Number of label sets: " << lss.series_count() << std::endl;
        log() << std::endl;
      }
    }

    auto new_checkpoint = lss.checkpoint();
    auto delta = new_checkpoint - checkpoint;
    if (lss.save_size(delta) > 1024ULL * 1024ULL) {
      log() << "WAL size: " << lss.save_size(delta) << std::endl;
      lss.save(out, delta);
      checkpoint = new_checkpoint;
    }
  }

  auto new_checkpoint = lss.checkpoint();
  auto delta = new_checkpoint - checkpoint;
  if (!delta.empty()) {
    log() << "WAL size: " << lss.save_size(delta) << std::endl;
    lss.save(out, delta);
  }

  auto now = std::chrono::steady_clock::now();

  metrics << (Metric() << "parallel_encoding_bimap_add_label_set_avg_duration_nanoseconds"
                       << static_cast<double>(std::chrono::duration_cast<std::chrono::nanoseconds>(now - start).count()) /
                              static_cast<double>(dummy_wal.cnt()));
}
