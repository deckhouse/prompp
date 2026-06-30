#include "head_status.h"

#include "entrypoint/types/data_storage.h"
#include "entrypoint/types/lss.h"
#include "head/status.h"
#include "primitives/go_slice.h"

using entrypoint_types::DataStoragePtr;
using entrypoint_types::LssVariantPtr;

using Status = head::Status<PromPP::Primitives::Go::String, PromPP::Primitives::Go::Slice>;

extern "C" void prompp_get_head_status_lss(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    size_t limit;
  };

  const auto in = static_cast<const Arguments*>(args);
  const auto& lss = std::get<entrypoint_types::QueryableEncodingBimap>(*in->lss);

  head::StatusGetterLSS<entrypoint_types::QueryableEncodingBimap, Status>{lss, in->limit}.get(*static_cast<Status*>(res));
}

extern "C" void prompp_get_head_status_data_storage(void* args, void* res) {
  struct Arguments {
    DataStoragePtr data_storage;
  };

  const auto in = static_cast<const Arguments*>(args);
  auto* status = static_cast<Status*>(res);

  status->min_max_timestamp = series_data::Decoder::get_time_interval(*in->data_storage);
  status->chunk_count = in->data_storage->chunks().non_empty_chunk_count();
}

extern "C" void prompp_free_head_status(void* args) {
  static_cast<Status*>(args)->~Status();
}
