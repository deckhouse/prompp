#include <benchmark/benchmark.h>

#include "benchmark/statistic.h"
#include "primitives/snug_composites.h"
#include "profiling/profiling.h"
#include "series_index/reverse_index.h"

namespace {

using BareBones::StreamVByte::CompactSequence;
using BareBones::StreamVByte::Sequence;

using EncodingBimap = PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<BareBones::Vector>;

struct Label {
  uint32_t ls_id_;
  uint32_t name_id_;
  uint32_t value_id_;

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t value_id() const noexcept { return value_id_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t name_id() const noexcept { return name_id_; }
};

std::string get_lss_file() {
  if (auto& context = benchmark::internal::GetGlobalContext(); context != nullptr) {
    return context->operator[]("lss_file");
  }

  return {};
}

BareBones::Vector<Label> get_labels() {
  static BareBones::Vector<Label> labels;
  if (labels.empty()) {
    EncodingBimap lss;
    if (lss.size() == 0) {
      std::ifstream infile(get_lss_file(), std::ios_base::binary);
      infile >> lss;
    }

    uint32_t series_id{};
    for (auto label_set : lss) {
      for (auto i = label_set.begin(); i != label_set.end(); ++i) {
        labels.emplace_back(Label{.ls_id_ = series_id, .name_id_ = i.name_id(), .value_id_ = i.value_id()});
      }

      ++series_id;
    }
  }

  return labels;
}

void GenerateReverseIndex(benchmark::State& state) {
  ZoneScoped;
  static const auto& labels = get_labels();
  static size_t allocated_memory = 0;

  for ([[maybe_unused]] auto _ : state) {
    series_index::SeriesReverseIndex series_reverse_index;

    for (const auto& label : labels) {
      series_reverse_index.add(label, label.ls_id_);
    }
  }

  if (allocated_memory == 0) [[unlikely]] {
    series_index::SeriesReverseIndex series_reverse_index;

    for (const auto& label : labels) {
      series_reverse_index.add(label, label.ls_id_);
    }

    allocated_memory = series_reverse_index.allocated_memory();
  }
  state.counters["Memory"] = benchmark::Counter(static_cast<double>(allocated_memory), benchmark::Counter::kDefaults, benchmark::Counter::kIs1024);
}

BENCHMARK(GenerateReverseIndex)->ComputeStatistics("min", benchmark::min_time);

}  // namespace
