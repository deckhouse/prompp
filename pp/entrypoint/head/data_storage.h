#pragma once

#include "bare_bones/jemalloc.h"
#include "series_data/data_storage.h"

namespace entrypoint::head {

using DataStorage = series_data::DataStorage<BareBones::jemalloc::ArenaReallocator>;
using DataStoragePtr = std::unique_ptr<DataStorage>;

static_assert(sizeof(DataStoragePtr) == sizeof(void*));

}  // namespace entrypoint::head
