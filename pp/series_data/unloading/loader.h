#pragma once

#include <roaring/roaring.hh>
#include "roaring/cpp/roaring.hh"

#include "series_data/data_storage.h"

namespace series_data::unloading {
class Loader {
 public:
  template <typename LsIDStorage>
  explicit Loader(DataStorage& storage, const LsIDStorage& ls_id_query) : storage_(storage) {
    for (const auto& ls_id : ls_id_query) {
      if (storage_.unused_series_bitmap.contains(ls_id)) {
        series_to_load_.add(ls_id);
        storage_.unused_series_bitmap.remove(ls_id);
      }
    }
    series_to_load_.runOptimize();
    series_to_load_.shrinkToFit();
  }

  void load_next(std::span<const uint8_t> buffer);
  void load_finalize();

 private:
  DataStorage& storage_;
  roaring::Roaring series_to_load_{};
};
}  // namespace series_data::unloading