#pragma once

#include "series_data/serialization/serialized_data.h"

namespace entrypoint::head {

using SerializedDataPtr = std::unique_ptr<series_data::serialization::SerializedData>;
using SerializedDataIteratorPtr = std::unique_ptr<series_data::serialization::SerializedData::SerializedSeriesIterator>;

static_assert(sizeof(SerializedDataPtr) == sizeof(void*));
static_assert(sizeof(SerializedDataIteratorPtr) == sizeof(void*));

}  // namespace entrypoint::head