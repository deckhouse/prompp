#pragma once

#include <memory>
#include <variant>

#include "series_data/data_storage.h"

namespace entrypoint::types {

enum DataStorageType : uint8_t {
  kWithArenas = 0,
  kWithoutArenas,
};

using DataStorageWithArenas = series_data::DataStorage<true>;
using DataStorageWithoutArenas = series_data::DataStorage<false>;
using DataStorageVariant = std::variant<DataStorageWithArenas, DataStorageWithoutArenas>;

using DataStoragePtr = std::unique_ptr<DataStorageVariant>;

static_assert(sizeof(DataStoragePtr) == sizeof(void*));

}  // namespace entrypoint::types
