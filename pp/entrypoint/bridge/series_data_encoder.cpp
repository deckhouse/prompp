#include "series_data_encoder.h"

#include <type_traits>

#include "entrypoint/types/data_storage.h"
#include "entrypoint/types/encoder.h"
#include "entrypoint/types/serialization.h"
#include "prometheus/relabeler.h"
#include "series_data/data_storage.h"

extern "C" void prompp_series_data_encoder_ctor(void* args, void* res) {
  struct Arguments {
    entrypoint::types::DataStoragePtr data_storage;
  };
  using Result = struct {
    entrypoint::types::SeriesDataEncoderWrapperPtr encoder_wrapper;
  };

  const auto in = static_cast<Arguments*>(args);
  std::visit(
      [res]<typename DataStorage>(DataStorage& data_storage) {
        using Wrapper = entrypoint::types::SeriesDataEncoderWrapper<std::remove_cvref_t<DataStorage>>;
        new (res) Result{.encoder_wrapper = std::make_unique<entrypoint::types::SeriesDataEncoderWrapperVariant>(std::in_place_type<Wrapper>, data_storage)};
      },
      *in->data_storage);
}

extern "C" void prompp_series_data_encoder_encode(void* args) {
  struct Arguments {
    entrypoint::types::SeriesDataEncoderWrapperPtr encoder_wrapper;
    uint32_t series_id;
    int64_t timestamp;
    double value;
  };

  const auto* in = static_cast<Arguments*>(args);
  std::visit(
      [in](auto& wrapper) {
        const auto arena_guard = wrapper.encoder.storage().thread_arena_guard();
        wrapper.encoder.encode(in->series_id, in->timestamp, in->value);
      },
      *in->encoder_wrapper);
}

extern "C" void prompp_series_data_encoder_encode_inner_series_slice(void* args) {
  struct Arguments {
    entrypoint::types::SeriesDataEncoderWrapperPtr encoder_wrapper;
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries> inner_series_slice;
  };

  auto* in = static_cast<Arguments*>(args);
  std::visit(
      [in](auto& wrapper) {
        const auto arena_guard = wrapper.encoder.storage().thread_arena_guard();

        std::ranges::for_each(in->inner_series_slice, [&](const PromPP::Prometheus::Relabel::InnerSeries& inner_series) {
          if (inner_series.size() == 0) {
            return;
          }

          std::ranges::for_each(inner_series.data(), [&](const PromPP::Prometheus::Relabel::InnerSerie& inner_serie) {
            wrapper.encoder.encode(inner_serie.ls_id, inner_serie.sample.timestamp(), inner_serie.sample.value());
          });
        });
      },
      *in->encoder_wrapper);
}

extern "C" void prompp_series_data_encoder_merge_out_of_order_chunks(void* args) {
  struct Arguments {
    entrypoint::types::SeriesDataEncoderWrapperPtr encoder_wrapper;
  };

  const auto in = static_cast<Arguments*>(args);
  std::visit(
      [](auto& wrapper) {
        const auto arena_guard = wrapper.encoder.storage().thread_arena_guard();
        series_data::OutdatedChunkMerger{wrapper.encoder}.merge();
      },
      *in->encoder_wrapper);
}

extern "C" void prompp_series_data_encoder_dtor(void* args) {
  struct Arguments {
    entrypoint::types::SeriesDataEncoderWrapperPtr encoder_wrapper;
  };

  static_cast<Arguments*>(args)->~Arguments();
}
