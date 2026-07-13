#include "series_data_encoder.h"
#include "annotations.h"

#include "entrypoint/types/encoder.h"
#include "primitives/primitives.h"
#include "prometheus/relabeler.h"
#include "series_data/data_storage.h"

/**
 * @brief series data Encoder constructor.
 *
 * @param args {
 *     data_storage uintptr // pointer to constructed data storage
 * }
 *
 * @param res {
 *     encoder uintptr // pointer to constructed encoder
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_encoder_ctor(void* args, void* res) {
  struct Arguments {
    series_data::DataStorage* data_storage;
  };
  using Result = struct {
    entrypoint::types::SeriesDataEncoderWrapperPtr encoder_wrapper;
  };

  new (res) Result{.encoder_wrapper = std::make_unique<entrypoint::types::SeriesDataEncoderWrapper>(*static_cast<Arguments*>(args)->data_storage)};
}

/**
 * @brief adds single series to data storage
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 *     seriesID uint32 // series id
 *     timestamp int64 // timestamp
 *     value float64   // value
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_encoder_encode(void* args) {
  struct Arguments {
    entrypoint::types::SeriesDataEncoderWrapperPtr encoder_wrapper;
    uint32_t series_id;
    int64_t timestamp;
    double value;
  };

  const auto* in = static_cast<Arguments*>(args);
  const auto arena_guard = in->encoder_wrapper->encoder.storage().thread_arena_guard();

  in->encoder_wrapper->encoder.encode(in->series_id, in->timestamp, in->value);
}

/**
 * @brief adds slice of inner series to data storage
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 *     innerSeriesSlice []*InnerSeries // pointer to inner series slice.
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_encoder_encode_inner_series_slice(void* args) {
  struct Arguments {
    entrypoint::types::SeriesDataEncoderWrapperPtr encoder_wrapper;
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries> inner_series_slice;
  };

  auto* in = static_cast<Arguments*>(args);
  const auto arena_guard = in->encoder_wrapper->encoder.storage().thread_arena_guard();

  std::ranges::for_each(in->inner_series_slice, [&](const PromPP::Prometheus::Relabel::InnerSeries& inner_series) {
    if (inner_series.size() == 0) {
      return;
    }

    std::ranges::for_each(inner_series.data(), [&](const PromPP::Prometheus::Relabel::InnerSerie& inner_serie) {
      in->encoder_wrapper->encoder.encode(inner_serie.ls_id, inner_serie.sample.timestamp(), inner_serie.sample.value());
    });
  });
}

/**
 * @brief merge outdated chunks
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_encoder_merge_out_of_order_chunks(void* args) {
  struct Arguments {
    entrypoint::types::SeriesDataEncoderWrapperPtr encoder_wrapper;
  };

  auto& encoder = static_cast<Arguments*>(args)->encoder_wrapper->encoder;
  const auto arena_guard = encoder.storage().thread_arena_guard();

  entrypoint::types::OutdatedChunkMerger{encoder}.merge();
}

/**
 * @brief series data Encoder destructor.
 *
 * @param args {
 *     encoder uintptr // pointer to constructed encoder
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_encoder_dtor(void* args) {
  struct Arguments {
    entrypoint::types::SeriesDataEncoderWrapperPtr encoder_wrapper;
  };

  static_cast<Arguments*>(args)->~Arguments();
}
