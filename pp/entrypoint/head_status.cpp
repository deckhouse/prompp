#include "head_status.h"

#include "head/data_storage.h"
#include "head/lss.h"
#include "head/status.h"
#include "primitives/go_slice.h"

using entrypoint::head::DataStoragePtr;
using entrypoint::head::LssVariantPtr;

using Status = head::Status<PromPP::Primitives::Go::String, PromPP::Primitives::Go::Slice>;

extern "C" void prompp_get_head_status_lss(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    size_t limit;
  };

  const auto in = static_cast<const Arguments*>(args);
  const auto& lss = std::get<entrypoint::head::QueryableEncodingBimap>(*in->lss);

  head::StatusGetterLSS<entrypoint::head::QueryableEncodingBimap, Status>{lss, in->limit}.get(*static_cast<Status*>(res));
}

extern "C" void prompp_get_head_status_data_storage(void* args, void* res) {
  struct Arguments {
    DataStoragePtr data_storage;
  };

  const auto in = static_cast<const Arguments*>(args);
  auto* status = static_cast<Status*>(res);

  status->min_max_timestamp = series_data::Decoder<entrypoint::head::DataStorage>::get_time_interval(*in->data_storage);
  status->chunk_count = in->data_storage->chunks().non_empty_chunk_count();
}

extern "C" void prompp_free_head_status(void* args) {
  static_cast<Status*>(args)->~Status();
}
