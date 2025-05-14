#pragma once

#include "series_data/data_storage.h"
#include "snapshot.h"

namespace series_data::snapshot {
class Unloader {
 public:
  explicit Unloader(DataStorage& storage) : storage_(storage) {}

  Snapshot unload() {
    Snapshot snapshot;
    for (const auto& ls_id : storage_.unused_series_bitmap) {
    }
    return snapshot;
  }

 private:
  DataStorage& storage_;
};
}  // namespace series_data::snapshot