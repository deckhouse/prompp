#pragma once

#include "metric.h"
#include "primitives/hash.h"
#include "prometheus/hashdex.h"

namespace PromPP::WAL::hashdex {

inline std::ostream& operator<<(std::ostream& stream, const FloatMetric& item) {
  stream << "hash: " << item.hash << ", labels: { ";

  for (const auto& [name, value] : item.timeseries.label_set()) {
    stream << name << " = " << value << ", ";
  }

  stream << " }, samples: { ";

  for (const auto& [timestamp, value] : item.timeseries.samples()) {
    stream << timestamp << " => " << value << ", ";
  }

  stream << " }";

  return stream;
}

inline std::ostream& operator<<(std::ostream& stream, const Metadata& item) {
  stream << "[" << static_cast<int>(item.type) << "] " << item.metric_name << ": " << item.text;
  return stream;
}

inline void calculate_labelset_hash(std::vector<FloatMetric>& floats) noexcept {
  for (auto& item : floats) {
    item.hash = Primitives::hash::hash_of_label_set(item.timeseries.label_set());
  }
}

inline void calculate_labelset_hash(std::vector<HistogramMetric>& histograms) noexcept {
  for (auto& item : histograms) {
    item.hash = Primitives::hash::hash_of_label_set(item.label_set);
  }
}

template <Prometheus::hashdex::HashdexInterface Hashdex>
[[nodiscard]] std::vector<FloatMetric> get_floats(const Hashdex& hashdex) noexcept {
  std::vector<FloatMetric> items;
  items.reserve(hashdex.floats().size());

  for (auto& item : hashdex.floats()) {
    auto& scraped_item = items.emplace_back(FloatMetric{.hash = item.hash()});
    item.read(scraped_item.timeseries);
  }

  return items;
}

template <Prometheus::hashdex::HashdexInterface Hashdex>
[[nodiscard]] std::vector<HistogramMetric> get_histograms(const Hashdex& hashdex) noexcept {
  std::vector<HistogramMetric> items;
  items.reserve(hashdex.histograms().size());

  for (auto& item : hashdex.histograms()) {
    auto& scraped_item = items.emplace_back();
    scraped_item.hash = item.hash();
    item.read(scraped_item);
  }

  return items;
}

template <Prometheus::hashdex::HashdexInterface Hashdex>
[[nodiscard]] std::vector<Metadata> get_metadata(const Hashdex& hashdex) noexcept {
  std::vector<Metadata> items;
  items.reserve(hashdex.metadata().size());

  for (auto& item : hashdex.metadata()) {
    if constexpr (std::is_same_v<decltype(item), const Metadata&>) {
      items.emplace_back(Metadata{.metric_name = item.metric_name, .text = item.text, .type = item.type});
    } else {
      items.emplace_back(Metadata{.metric_name = item.metric_name(), .text = item.text(), .type = item.type()});
    }
  }

  return items;
}

}  // namespace PromPP::WAL::hashdex