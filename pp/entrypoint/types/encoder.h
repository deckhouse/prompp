#pragma once

#include <memory>
#include <variant>

#include "series_data/data_storage.h"
#include "series_data/encoder.h"
#include "series_data/outdated_chunk_merger.h"

namespace entrypoint::types {

template <class Storage>
struct SeriesDataEncoderWrapper {
  using Encoder = series_data::Encoder<Storage>;

  Encoder encoder;

  explicit SeriesDataEncoderWrapper(Storage& data_storage) : encoder{data_storage} {}
};

using SeriesDataEncoderWrapperVariant =
    std::variant<SeriesDataEncoderWrapper<series_data::DataStorage<true>>, SeriesDataEncoderWrapper<series_data::DataStorage<false>>>;
using SeriesDataEncoderWrapperPtr = std::unique_ptr<SeriesDataEncoderWrapperVariant>;

static_assert(sizeof(SeriesDataEncoderWrapperPtr) == sizeof(void*));

}  // namespace entrypoint::types
