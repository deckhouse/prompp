#pragma once

#include "data_storage.h"
#include "series_data/encoder.h"
#include "series_data/outdated_chunk_merger.h"

namespace entrypoint::head {

using Encoder = series_data::Encoder<DataStorage>;
using OutdatedChunkMerger = series_data::OutdatedChunkMerger<Encoder>;

struct SeriesDataEncoderWrapper {
  Encoder encoder;

  explicit SeriesDataEncoderWrapper(DataStorage& data_storage) : encoder{data_storage} {}
};

using SeriesDataEncoderWrapperPtr = std::unique_ptr<SeriesDataEncoderWrapper>;

static_assert(sizeof(SeriesDataEncoderWrapperPtr) == sizeof(void*));

}  // namespace entrypoint::head