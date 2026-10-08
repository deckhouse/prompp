#pragma once

#include <cstdint>
#include <limits>

#include "bare_bones/gorilla.h"

namespace series_data::encoder::timestamp {

using SequenceId = uint32_t;
using TimestampEncoderCodec = BareBones::Encoding::Gorilla::ZigZagTimestampEncoder<>;
using TimestampDecoderCodec = BareBones::Encoding::Gorilla::ZigZagTimestampDecoder<>;

static constexpr auto kInvalidSequenceId = std::numeric_limits<SequenceId>::max();

}  // namespace series_data::encoder::timestamp
