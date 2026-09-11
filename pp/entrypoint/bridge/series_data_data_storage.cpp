#include "series_data_data_storage.h"

#include <cassert>
#include <ranges>
#include <span>
#include <spanstream>

#include "entrypoint/types/data_storage.h"
#include "entrypoint/types/loader.h"
#include "entrypoint/types/lss.h"
#include "entrypoint/types/querier.h"
#include "entrypoint/types/serialization.h"
#include "head/chunk_recoder.h"
#include "primitives/go_slice.h"
#include "series_data/data_storage.h"
#include "series_data/decoder.h"
#include "series_data/querier/instant_querier.h"
#include "series_data/querier/querier.h"
#include "series_data/unloading/loader.h"
#include "series_data/unloading/unloader.h"
#include "series_index/querier/selector_querier.h"

using entrypoint::types::DataStoragePtr;
using entrypoint::types::DataStorageType;
using entrypoint::types::DataStorageVariant;
using entrypoint::types::DataStorageWithArenas;
using entrypoint::types::QueryableEncodingBimap;
using entrypoint::types::QueryStatus;
using PromPP::Primitives::LabelSetID;
using PromPP::Primitives::Go::BytesStream;
using PromPP::Primitives::Go::Slice;
using PromPP::Primitives::Go::SliceView;
using series_data::DataStorage;
using ChunkRecoderIterator = head::ChunkRecoderIterator<QueryableEncodingBimap::LsIdSetIterator, QueryableEncodingBimap::LsIdSetIterator>;
using ChunkRecoder = head::ChunkRecoder<ChunkRecoderIterator>;

using SerializedChunkRecoder = head::ChunkRecoder<series_data::chunk::SerializedChunkIterator>;

using ChunkRecoderVariant = std::variant<ChunkRecoder, SerializedChunkRecoder>;
using ChunkRecoderVariantPtr = std::unique_ptr<ChunkRecoderVariant>;

using entrypoint::types::RevertableLoader;

using LoaderVariant = std::variant<series_data::unloading::Loader<>, RevertableLoader>;
using LoaderVariantPtr = std::unique_ptr<LoaderVariant>;
static_assert(sizeof(LoaderVariantPtr) == sizeof(void*));

using entrypoint::types::QuerierType;
using entrypoint::types::QuerierVariant;
using entrypoint::types::QuerierVariantPtr;

extern "C" void prompp_series_data_data_storage_ctor(void* args, void* res) {
  struct Arguments {
    bool collect_metrics;
    bool use_arenas;
  };
  using Result = struct {
    DataStoragePtr data_storage;
  };

  if (const auto in = static_cast<Arguments*>(args); in->use_arenas) [[unlikely]] {
    new (res) Result{.data_storage = std::make_unique<DataStorageVariant>(std::in_place_index<DataStorageType::kWithArenas>, in->collect_metrics)};
  } else {
    new (res) Result{.data_storage = std::make_unique<DataStorageVariant>(std::in_place_index<DataStorageType::kWithoutArenas>, in->collect_metrics)};
  }
}

extern "C" void prompp_series_data_data_storage_time_interval(void* args, void* res) {
  struct Arguments {
    DataStoragePtr data_storage;
  };
  struct Result {
    PromPP::Primitives::TimeInterval interval;
  };

  std::visit([res](const auto& data_storage) { new (res) Result{.interval = series_data::Decoder::get_time_interval(data_storage)}; },
             *static_cast<Arguments*>(args)->data_storage);
}

extern "C" void prompp_series_data_data_storage_queried_series_bitset_size(void* args, void* res) {
  struct Arguments {
    DataStoragePtr data_storage;
  };
  struct Result {
    uint32_t size;
  };

  std::visit([res](const auto& data_storage) { new (res) Result{.size = data_storage.queried_series_bitmap.get_write_size()}; },
             *static_cast<Arguments*>(args)->data_storage);
}

extern "C" void prompp_series_data_data_storage_queried_series_bitset(void* args, void* res) {
  struct Arguments {
    DataStoragePtr data_storage;
  };
  struct Result {
    Slice<char> queried_series;
  };

  BytesStream stream(&static_cast<Result*>(res)->queried_series);
  std::visit([&stream](const auto& data_storage) { data_storage.queried_series_bitmap.write_to(stream); }, *static_cast<Arguments*>(args)->data_storage);
}

extern "C" void prompp_series_data_data_storage_queried_series_set_bitset(void* args, void* res) {
  struct Arguments {
    DataStoragePtr data_storage;
    SliceView<char> queried_series;
  };
  struct Result {
    bool result;
  };

  const auto in = static_cast<Arguments*>(args);
  std::ispanstream stream(in->queried_series.span());

  std::visit(
      [&stream, res](auto& data_storage) {
        [[maybe_unused]] const auto arena_guard = data_storage.thread_arena_guard();
        const auto result = data_storage.queried_series_bitmap.read_from(stream);
        if (!result) {
          data_storage.queried_series_bitmap.reset(0);
        }
        new (res) Result{.result = result};
      },
      *in->data_storage);
}

extern "C" void prompp_get_promql_optimized_functions(void* res) {
  using PromPP::Prometheus::promql::FunctionType;
  using PromPP::Prometheus::promql::kFunctions;

  struct GoFunction {
    PromPP::Primitives::Go::String name;
    FunctionType type;
  };

  static constexpr auto kGoFunctions = [] {
    std::array<GoFunction, kFunctions.size()> functions;
    for (size_t i = 0; i < functions.size(); ++i) {
      functions[i] = {.name = PromPP::Primitives::Go::String(kFunctions[i].name), .type = kFunctions[i].type};
    }
    return functions;
  }();

  struct Result {
    SliceView<const GoFunction> functions;
  };

  new (res) Result{.functions = SliceView{kGoFunctions.data(), kGoFunctions.size(), kGoFunctions.size()}};
}

extern "C" void prompp_series_data_data_storage_query_v2(void* args, void* res) {
  using Query = series_data::querier::Query<Slice<LabelSetID>>;

  struct Arguments {
    DataStoragePtr data_storage;
    Query query;
    PromPP::Primitives::Timestamp downsampling_ms;
    entrypoint::types::GoSelectHints* hints;
  };

  struct Result {
    QuerierVariantPtr querier{};
    QueryStatus status{};
    entrypoint::types::SerializedDataPtr* serialized_data{};
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = static_cast<Result*>(res);

  std::visit(
      [in, out]<typename DataStorage>(DataStorage& data_storage) {
        using Querier = entrypoint::types::BasicRangeQuerierWithArgumentsWrapperV2<DataStorage>;
        Querier querier(data_storage, in->query, in->hints ? *in->hints : entrypoint::types::GoSelectHints{}, out->serialized_data, in->downsampling_ms);
        querier.query();

        if (querier.need_loading()) {
          out->querier = std::make_unique<QuerierVariant>(std::in_place_type<Querier>, std::move(querier));
          out->status = QueryStatus::kNeedDataLoad;
        } else {
          out->status = QueryStatus::kSuccess;
        }
      },
      *in->data_storage);
}

extern "C" void prompp_series_data_serialized_data_next(void* args, void* res) {
  struct Arguments {
    entrypoint::types::SerializedDataPtr serialized_data;
  };

  using Result = struct {
    uint32_t series_id;
    uint32_t chunk_ref;
  };
  const auto out = new (res) Result{};
  std::visit([out](auto& serialized_data) { std::tie(out->series_id, out->chunk_ref) = serialized_data.next(); },
             *static_cast<Arguments*>(args)->serialized_data);
}

extern "C" void prompp_series_data_serialized_data_dtor(void* args) {
  struct Arguments {
    entrypoint::types::SerializedDataPtr serialized_data;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_series_data_data_storage_instant_query(void* args, void* res) {
  using PromPP::Primitives::Timestamp;

  struct Arguments {
    DataStoragePtr data_storage;
    SliceView<LabelSetID> label_set_ids;
    Timestamp timestamp;
    entrypoint::types::SampleWithGoLabels* samples;
  };

  using Result = struct {
    QuerierVariantPtr querier;
    QueryStatus status;
  };

  const auto in = static_cast<Arguments*>(args);

  auto samples = std::span(in->samples, in->label_set_ids.size());
  std::visit(
      [in, res, samples]<typename DataStorage>(DataStorage& data_storage) mutable {
        using Querier =
            entrypoint::types::InstantQuerierWithArgumentsWrapper<SliceView<LabelSetID>, std::span<entrypoint::types::SampleWithGoLabels>, DataStorage>;
        Querier instant_querier(data_storage, in->label_set_ids, in->timestamp, samples);
        instant_querier.query();

        if (instant_querier.need_loading()) {
          new (res) Result{
              .querier = std::make_unique<QuerierVariant>(std::in_place_type<Querier>, std::move(instant_querier)),
              .status = QueryStatus::kNeedDataLoad,
          };
        } else {
          new (res) Result{.querier = nullptr, .status = QueryStatus::kSuccess};
        }
      },
      *in->data_storage);
}

extern "C" void prompp_series_data_data_storage_query_final(void* args) {
  using entrypoint::types::QuerierVariantPtr;

  struct Arguments {
    Slice<QuerierVariantPtr> queriers;
  };

  const auto in = static_cast<Arguments*>(args);
  for (auto& querier_ptr : in->queriers) {
    std::visit([](auto& querier) { querier.query_finalize(); }, *querier_ptr);
    querier_ptr.reset();
  }
}

extern "C" void prompp_series_data_data_storage_query_stalenan_series(void* args) {
  using series_data::Decoder;

  struct Arguments {
    DataStoragePtr data_storage;
    SliceView<LabelSetID> series_ids;
    entrypoint::types::StaleNaNSeriesWithGoLabels* series;
  };

  const auto in = static_cast<Arguments*>(args);
  auto series = std::span(in->series, in->series_ids.size());
  std::visit(
      [in, series](const auto& data_storage) mutable {
        for (auto&& [stalenan_series, series_id] : std::ranges::views::zip(series, in->series_ids)) {
          stalenan_series.series_id = series_id;
          if (data_storage.series_exists(series_id)) [[likely]] {
            stalenan_series.timestamp = Decoder::get_series_min_timestamp(data_storage, series_id);
          }
        }
      },
      *in->data_storage);
}

extern "C" void prompp_series_data_data_storage_allocated_memory(void* args, void* res) {
  struct Arguments {
    DataStoragePtr data_storage;
  };

  struct Result {
    uint64_t allocated_memory;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = new (res) Result();

  std::visit([out](const auto& data_storage) { out->allocated_memory = data_storage.allocated_memory(); }, *in->data_storage);
}

extern "C" void prompp_series_data_data_storage_dtor(void* args) {
  struct Arguments {
    DataStoragePtr data_storage;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_series_data_chunk_recoder_ctor(void* args, void* res) {
  struct Arguments {
    entrypoint::types::LssVariantPtr lss;
    uint32_t ls_id_batch_size;
    DataStoragePtr data_storage;
    PromPP::Primitives::TimeInterval time_interval;
    PromPP::Primitives::Timestamp downsampling_ms;
  };
  struct Result {
    ChunkRecoderVariantPtr chunk_recoder;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto& ls_id_set = std::get<QueryableEncodingBimap>(*in->lss).ls_id_set();

  new (res) Result{
      .chunk_recoder = std::make_unique<ChunkRecoderVariant>(std::in_place_type<ChunkRecoder>,
                                                             ChunkRecoderIterator{ls_id_set.begin(), ls_id_set.end(), in->ls_id_batch_size,
                                                                                  &std::get<DataStorageWithArenas>(*in->data_storage), in->time_interval},
                                                             in->time_interval, in->downsampling_ms),
  };
}

extern "C" void prompp_series_data_serialized_chunk_recoder_ctor(void* args, void* res) {
  struct Arguments {
    entrypoint::types::SerializedDataPtr* serialized_data;
    PromPP::Primitives::TimeInterval time_interval;
  };
  struct Result {
    ChunkRecoderVariantPtr chunk_recoder;
  };

  const auto in = static_cast<Arguments*>(args);
  std::visit(
      [in, res](auto& serialized_data) {
        new (res) Result{
            .chunk_recoder = std::make_unique<ChunkRecoderVariant>(
                std::in_place_type<SerializedChunkRecoder>,
                series_data::chunk::SerializedChunkIterator{serialized_data.get_buffer_view(), serialized_data.get_chunks_view()}, in->time_interval,
                series_data::decoder::decorator::kNoDownsampling),
        };
      },
      **in->serialized_data);
}

extern "C" void prompp_series_data_chunk_recoder_recode_next_chunk(void* args, void* res) {
  struct Arguments {
    ChunkRecoderVariantPtr chunk_recoder;
  };
  struct Result {
    PromPP::Primitives::TimeInterval interval;
    uint32_t series_id;
    uint8_t samples_count;
    bool has_more_data;
    SliceView<const uint8_t> buffer;
  };

  const auto in = static_cast<const Arguments*>(args);
  const auto out = static_cast<Result*>(res);
  // NOLINTNEXTLINE(clang-analyzer-core.NullDereference)
  std::visit(
      [out](auto& chunk_recoder) PROMPP_LAMBDA_INLINE {
        chunk_recoder.recode_next_chunk(*out);
        out->has_more_data = chunk_recoder.has_more_data();
        out->buffer.reset_to(chunk_recoder.bytes());
      },
      *in->chunk_recoder);
}

extern "C" void prompp_series_data_chunk_recoder_next_batch(void* args, void* res) {
  struct Arguments {
    ChunkRecoderVariantPtr chunk_recoder;
  };
  struct Result {
    bool has_more_data;
  };

  auto& recoder = std::get<ChunkRecoder>(*static_cast<const Arguments*>(args)->chunk_recoder);
  new (res) Result{.has_more_data = recoder.chunk_iterator().next_batch()};
}

extern "C" void prompp_series_data_chunk_recoder_dtor(void* args) {
  struct Arguments {
    ChunkRecoderVariantPtr chunk_recoder;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

struct Unloader {
  explicit Unloader(DataStorageWithArenas& storage) : unloader(storage) {}

  series_data::unloading::Unloader<DataStorageWithArenas> unloader;
  Slice<char> snapshot;
};

using UnloaderPtr = std::unique_ptr<Unloader>;
static_assert(sizeof(UnloaderPtr) == sizeof(void*));

extern "C" void prompp_series_data_data_storage_unloader_ctor(void* args, void* res) {
  struct Arguments {
    DataStoragePtr data_storage;
  };

  struct Result {
    UnloaderPtr unloader;
  };

  new (res) Result{.unloader = std::make_unique<Unloader>(std::get<DataStorageWithArenas>(*static_cast<Arguments*>(args)->data_storage))};
}

extern "C" void prompp_series_data_data_storage_unloader_dtor(void* args) {
  struct Arguments {
    UnloaderPtr unloader;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_series_data_data_storage_unloader_create_snapshot(void* args, void* res) {
  struct Arguments {
    UnloaderPtr unloader;
  };

  struct Result {
    SliceView<char> snapshot;
  };

  auto& unloader = *static_cast<Arguments*>(args)->unloader;
  unloader.snapshot.resize(0);
  BytesStream bytes_stream{&unloader.snapshot};
  unloader.unloader.create_snapshot(bytes_stream);

  const auto out = static_cast<Result*>(res);
  out->snapshot.reset_to(unloader.snapshot);
}

extern "C" void prompp_series_data_data_storage_unloader_unload(void* args) {
  struct Arguments {
    UnloaderPtr unloader;
  };

  auto& unloader = static_cast<const Arguments*>(args)->unloader->unloader;
  const auto arena_guard = unloader.storage().thread_arena_guard();

  unloader.unload();
}

extern "C" void prompp_series_data_data_storage_loader_ctor(void* args, void* res) {
  using series_data::unloading::Loader;

  struct Arguments {
    DataStoragePtr data_storage;
    SliceView<QuerierVariantPtr> queriers;
  };

  struct Result {
    LoaderVariantPtr loader;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = new (res)
      Result{.loader = std::make_unique<LoaderVariant>(std::in_place_type<Loader<DataStorageWithArenas>>, std::get<DataStorageWithArenas>(*in->data_storage))};
  auto& loader = std::get<Loader<>>(*out->loader);

  for (const auto& rest : in->queriers) {
    std::visit(
        [&loader](auto& querier) {
          const auto& series_to_load = querier.series_to_load();
          loader.add_series_to_load(series_to_load, series_to_load.popcount());
        },
        *rest);
  }
}

extern "C" void prompp_series_data_data_storage_revertable_loader_ctor(void* args, void* res) {
  struct Arguments {
    entrypoint::types::LssVariantPtr lss;
    uint32_t ls_id_batch_size;
    DataStoragePtr data_storage;
  };

  struct Result {
    LoaderVariantPtr loader;
  };

  const auto in = static_cast<Arguments*>(args);
  auto& ls_id_set = std::get<QueryableEncodingBimap>(*in->lss).ls_id_set();
  new (res) Result{
      .loader = std::make_unique<LoaderVariant>(std::in_place_type<RevertableLoader>, std::get<DataStorageWithArenas>(*in->data_storage), ls_id_set.begin(),
                                                ls_id_set.end(), in->ls_id_batch_size),
  };
}

extern "C" void prompp_series_data_data_storage_loader_load_next(void* args) {
  struct Arguments {
    LoaderVariantPtr loader;
    SliceView<const uint8_t> buffer;
    bool is_final;
  };

  const auto in = static_cast<Arguments*>(args);

  std::visit(
      [in](auto& loader) {
        const auto arena_guard = loader.storage().thread_arena_guard();

        loader.load_next(in->buffer.span());

        if (in->is_final) {
          loader.load_finalize();
        }
      },
      *in->loader);
}

extern "C" void prompp_series_data_data_storage_revertable_loader_next_batch(void* args, void* res) {
  struct Arguments {
    LoaderVariantPtr loader;
  };
  struct Result {
    bool has_more_data;
  };

  auto& loader = std::get<RevertableLoader>(*static_cast<const Arguments*>(args)->loader);
  const auto arena_guard = loader.storage().thread_arena_guard();

  loader.revert();
  new (res) Result{.has_more_data = loader.next_batch()};
}

extern "C" void prompp_series_data_data_storage_loader_dtor(void* args) {
  struct Arguments {
    LoaderVariantPtr loader;
  };

  static_cast<Arguments*>(args)->~Arguments();
}
