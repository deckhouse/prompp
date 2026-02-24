#pragma once

#include "series_data/data_storage.h"

namespace entrypoint::head {

using DataStorage = series_data::DataStorage<>;
using DataStoragePtr = std::unique_ptr<DataStorage>;

static_assert(sizeof(DataStoragePtr) == sizeof(void*));

}  // namespace entrypoint::head
