#pragma once

#include <cstdint>
#include <string_view>

#include "primitives/sample.h"
#include "prometheus/metric.h"
#include "prometheus/value.h"

namespace PromPP::WAL::hashdex::scraper::inline marked {

#pragma pack(push, 1)
struct MarkedString {
  // Absolute offset into the scrape buffer. 64-bit so that scrape/federate responses
  // larger than 4 GiB stay addressable (label offsets are rebased to the metric line
  // before being encoded, so LabelCodec keeps its compact 32-bit representation).
  uint64_t offset = 0;
  uint32_t length = 0;

  [[nodiscard]] PROMPP_ALWAYS_INLINE static MarkedString create(std::string_view value, std::string_view buffer) noexcept {
    return {
        .offset = static_cast<uint64_t>(value.data() - buffer.data()),
        .length = static_cast<uint32_t>(value.size()),
    };
  }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_reserved_name() const noexcept { return offset == 0 && length == 0; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_empty() const noexcept { return length == 0; }

  [[nodiscard]] std::string_view view(const std::string_view& buffer) const noexcept {
    if (is_reserved_name()) [[unlikely]] {
      return Prometheus::kMetricLabelName;
    }

    return buffer.substr(offset, length);
  }
};

struct MarkedLabel {
  MarkedString name{};
  MarkedString value;
};

struct MarkedSample {
  Primitives::Sample sample{};
  bool has_ts{};
};

struct MarkedMetric {
  uint64_t hash;
  // Absolute offset of the metric line into the scrape buffer; 64-bit for > 4 GiB responses.
  uint64_t base_offset;
  // Offset into the per-scrape markup byte buffer; kept 32-bit and guarded (see
  // MetricMarkupBuffer::add_metric / MarkupBufferOverflowError).
  uint32_t data_offset;
};

struct MarkedMetadata {
  MarkedString metric_name{};
  MarkedString text{};
  Prometheus::MetadataType type{};
};
#pragma pack(pop)

}  // namespace PromPP::WAL::hashdex::scraper::inline marked