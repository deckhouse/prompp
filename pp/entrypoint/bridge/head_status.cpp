#include "head_status.h"
#include "annotations.h"

#include "entrypoint/types/data_storage.h"
#include "entrypoint/types/lss.h"
#include "head/status.h"
#include "primitives/go_slice.h"

namespace {

using entrypoint::types::DataStoragePtr;
using entrypoint::types::LssVariantPtr;

using Status = head::Status<PromPP::Primitives::Go::String, PromPP::Primitives::Go::Slice>;

}  // namespace

/**
 * @brief Return head status from lss.
 *
 * @param args {
 *     lss         uintptr      // pointer to constructed lss
 *     limit       int          // statistics limit
 * }
 *
 * @param res {
 *     status struct {          // head status
 *          label_value_count_by_label_name []struct {
 *              name string
 *              count uint32
 *          }
 *          series_count_by_metric_name []struct {
 *              name string
 *              count uint32
 *          }
 *          memory_in_bytes_by_label_name []struct {
 *              name string
 *              size uint32
 *          }
 *          series_count_by_label_value_pair [] struct {
 *              name string
 *              value string
 *              count uint32
 *          }
 *          num_series      uint32
 *          num_label_pairs uint32
 *     }
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_get_head_status_lss(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    size_t limit;
  };
  using Result = Status;

  const auto in = static_cast<const Arguments*>(args);
  const auto& lss = std::get<entrypoint::types::QueryableEncodingBimap>(*in->lss);

  head::StatusGetterLSS<entrypoint::types::QueryableEncodingBimap, Status>{lss, in->limit}.get(*static_cast<Result*>(res));
}

/**
 * @brief Return head status from lss.
 *
 * @param args {
 *     dataStorage uintptr      // pointer to constructed data storage
 * }
 *
 * @param res {
 *     status struct {          // head status
 *          time_interval struct {
 *              min int64
 *              max int64
 *          }
 *          chunk_count     uint32
 *     }
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_get_head_status_data_storage(void* args, void* res) {
  struct Arguments {
    DataStoragePtr data_storage;
  };
  using Result = Status;

  const auto in = static_cast<const Arguments*>(args);
  auto* status = static_cast<Result*>(res);

  status->min_max_timestamp = series_data::Decoder::get_time_interval(*in->data_storage);
  status->chunk_count = in->data_storage->chunks().non_empty_chunk_count();
}

/**
 * @brief Return head status
 *
 * @param args {
 *     status struct {...} // status returned by prompp_get_head_status
 * }
 *
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_free_head_status(void* args) {
  using Arguments = Status;

  static_cast<Arguments*>(args)->~Status();
}
