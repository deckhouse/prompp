#pragma once

#include <fstream>

#include <benchmark/benchmark.h>

#include "bare_bones/vector.h"
#include "primitives/sample.h"

namespace benchmark {

inline const auto& get_compact_samples() {
  static PromPP::Primitives::Timestamp timestamp_offset;

  class PROMPP(packed) CompactSample {
   public:
    [[nodiscard]] PROMPP(inline) PromPP::Primitives::Sample::value_type value() const noexcept { return value_; }
    [[nodiscard]] PROMPP(inline) PromPP::Primitives::Timestamp timestamp() const noexcept { return timestamp_ + timestamp_offset; }
    [[nodiscard]] PROMPP(inline) PromPP::Primitives::LabelSetID series_id() const noexcept { return series_id_; }

   private:
    PromPP::Primitives::Sample::value_type value_{};
    uint32_t timestamp_{};
    PromPP::Primitives::LabelSetID series_id_{};
  };

  static BareBones::Vector<CompactSample> samples;
  if (samples.empty()) {
    std::string filename;
    if (auto& context = internal::GetGlobalContext(); context != nullptr) {
      filename = context->operator[]("compact_samples_file");
      timestamp_offset = std::strtoll(context->operator[]("compact_samples_ts_offset").data(), nullptr, 10);
    }

    std::ifstream stream(filename, std::ios::binary);
    stream >> samples;
  }

  return samples;
}

}  // namespace benchmark
