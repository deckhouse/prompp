#pragma once

#include "bare_bones/preprocess.h"
#include "series_data/encoder/outdated.h"

namespace series_data::chunk {

class PROMPP_ATTRIBUTE_PACKED OutdatedChunk {
 public:
  OutdatedChunk(int64_t timestamp, double value) : encoder_(timestamp, value) {}

  PROMPP_ALWAYS_INLINE void encode(int64_t timestamp, double value) { encoder_.encode(timestamp, value); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE const encoder::CompactBitSequence& stream() const noexcept { return encoder_.stream(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept { return encoder_.allocated_memory(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t samples_count() const noexcept { return encoder_.samples_count(); }

 private:
  encoder::OutdatedEncoder encoder_;
};

}  // namespace series_data::chunk