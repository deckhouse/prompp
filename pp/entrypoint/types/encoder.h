#pragma once

#include <memory>

#include "series_data/data_storage.h"
#include "series_data/encoder.h"
#include "series_data/outdated_chunk_merger.h"

namespace entrypoint_types {

using Encoder = series_data::Encoder<>;
using OutdatedChunkMerger = series_data::OutdatedChunkMerger<Encoder>;

struct SeriesDataEncoderWrapper {
  Encoder encoder;

  explicit SeriesDataEncoderWrapper(series_data::DataStorage& data_storage) : encoder{data_storage} {}
};

using SeriesDataEncoderWrapperPtr = std::unique_ptr<SeriesDataEncoderWrapper>;

static_assert(sizeof(SeriesDataEncoderWrapperPtr) == sizeof(void*));

}  // namespace entrypoint_types
