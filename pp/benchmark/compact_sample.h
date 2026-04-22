#pragma once

#include <benchmark/benchmark.h>

#include "bare_bones/preprocess.h"
#include "bare_bones/vector.h"
#include "primitives/sample.h"

namespace benchmark {

inline PromPP::Primitives::Timestamp kTimestampOffset{};

class PROMPP_ATTRIBUTE_PACKED CompactSample {
 public:
  PROMPP_ALWAYS_INLINE PromPP::Primitives::Sample::value_type value() const noexcept { return value_; }
  PROMPP_ALWAYS_INLINE PromPP::Primitives::Timestamp timestamp() const noexcept { return timestamp_ + kTimestampOffset; }
  PROMPP_ALWAYS_INLINE PromPP::Primitives::LabelSetID series_id() const noexcept { return series_id_; }

 private:
  PromPP::Primitives::Sample::value_type value_{};
  uint32_t timestamp_{};
  PromPP::Primitives::LabelSetID series_id_{};
};

inline const BareBones::Vector<CompactSample>& get_compact_samples() {
  static BareBones::Vector<CompactSample> samples;
  if (samples.empty()) {
    std::string filename;
    if (auto& context = internal::GetGlobalContext(); context != nullptr) {
      filename = context->operator[]("compact_samples_file");
      kTimestampOffset = std::strtoul(context->operator[]("compact_samples_ts_offset").data(), nullptr, 10);
    }

    std::ifstream stream(filename, std::ios::binary);
    stream >> samples;
  }

  return samples;
}

}  // namespace benchmark