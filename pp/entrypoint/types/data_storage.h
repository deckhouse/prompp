#pragma once

#include <memory>

#include "series_data/data_storage.h"

namespace entrypoint::types {

using DataStoragePtr = std::unique_ptr<series_data::DataStorage>;

static_assert(sizeof(DataStoragePtr) == sizeof(void*));

}  // namespace entrypoint::types
