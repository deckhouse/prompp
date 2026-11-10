#include "series_data_encoder.h"
#include "annotations.hpp"

#include "entrypoint/types/data_storage.h"
#include "prometheus/relabeler.h"
#include "series_data/encoder.h"
#include "series_data/outdated_chunk_merger.h"

/**
 * @brief adds single series sample to data storage
 *
 * @param args {
 *     data_storage uintptr // pointer to constructed data storage
 *     seriesID uint32 // series id
 *     timestamp int64 // timestamp
 *     value float64   // value
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_encoder_encode(void* args) {
  struct Arguments {
    entrypoint::types::DataStoragePtr data_storage;
    uint32_t series_id;
    int64_t timestamp;
    double value;
  };

  const auto* in = static_cast<Arguments*>(args);
  std::visit(
      [in](auto& storage) {
        [[maybe_unused]] const auto arena_guard = storage.thread_arena_guard();
        series_data::Encoder{storage}.encode(in->series_id, in->timestamp, in->value);
      },
      *in->data_storage);
}

/**
 * @brief adds slice of inner series to data storage
 *
 * @param args {
 *     data_storage uintptr // pointer to constructed data storage
 *     innerSeriesSlice []*InnerSeries // pointer to inner series slice.
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_encoder_encode_inner_series_slice(void* args) {
  struct Arguments {
    entrypoint::types::DataStoragePtr data_storage;
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries> inner_series_slice;
  };

  auto* in = static_cast<Arguments*>(args);
  std::visit(
      [in](auto& storage) {
        [[maybe_unused]] const auto arena_guard = storage.thread_arena_guard();
        series_data::Encoder encoder{storage};

        std::ranges::for_each(in->inner_series_slice, [&](const PromPP::Prometheus::Relabel::InnerSeries& inner_series) {
          if (inner_series.size() == 0) {
            return;
          }

          std::ranges::for_each(inner_series.data(), [&](const PromPP::Prometheus::Relabel::InnerSerie& inner_serie) {
            encoder.encode(inner_serie.ls_id, inner_serie.sample.timestamp(), inner_serie.sample.value());
          });
        });
      },
      *in->data_storage);
}

/**
 * @brief merge outdated chunks
 *
 * @param args {
 *     data_storage uintptr // pointer to constructed data storage
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_encoder_merge_out_of_order_chunks(void* args) {
  struct Arguments {
    entrypoint::types::DataStoragePtr data_storage;
  };

  const auto in = static_cast<Arguments*>(args);
  std::visit(
      [](auto& storage) {
        [[maybe_unused]] const auto arena_guard = storage.thread_arena_guard();
        series_data::Encoder encoder{storage};
        series_data::OutdatedChunkMerger{encoder}.merge();
      },
      *in->data_storage);
}
