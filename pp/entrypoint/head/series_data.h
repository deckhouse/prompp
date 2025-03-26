#pragma once

#include <chrono>

#include "series_data/encoder.h"
#include "series_data/outdated_chunk_merger.h"
#include "series_data/outdated_sample_encoder.h"

namespace entrypoint::head {

using OutdatedSampleEncoder = series_data::OutdatedSampleEncoder<>;
using Encoder = series_data::Encoder<OutdatedSampleEncoder>;
using OutdatedChunkMerger = series_data::OutdatedChunkMerger<Encoder>;

struct SeriesDataEncoderWrapper {
  OutdatedSampleEncoder outdated_sample_encoder;
  Encoder encoder;

  explicit SeriesDataEncoderWrapper(series_data::DataStorage& data_storage) : encoder{data_storage, outdated_sample_encoder} {}
};

using SeriesDataEncoderWrapperPtr = std::unique_ptr<SeriesDataEncoderWrapper>;

static_assert(sizeof(SeriesDataEncoderWrapperPtr) == sizeof(void*));

}  // namespace entrypoint::head