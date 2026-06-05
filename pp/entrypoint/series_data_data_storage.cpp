#include "series_data_data_storage.h"

#include <cassert>
#include <spanstream>

#include "head/chunk_recoder.h"
#include "head/data_storage.h"
#include "head/lss.h"
#include "primitives/go_slice.h"
#include "series_data/data_storage.h"
#include "series_data/decoder.h"
#include "series_data/loader.h"
#include "series_data/querier.h"
#include "series_data/querier/instant_querier.h"
#include "series_data/querier/querier.h"
#include "series_data/serialization.h"
#include "series_data/unloading/loader.h"
#include "series_data/unloading/unloader.h"
#include "series_index/querier/selector_querier.h"

using entrypoint::head::DataStoragePtr;
using entrypoint::head::QueryableEncodingBimap;
using entrypoint::series_data::QueryStatus;
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

using entrypoint::series_data::RevertableLoader;

using LoaderVariant = std::variant<series_data::unloading::Loader, RevertableLoader>;
using LoaderVariantPtr = std::unique_ptr<LoaderVariant>;
static_assert(sizeof(LoaderVariantPtr) == sizeof(void*));

using entrypoint::series_data::QuerierType;
using entrypoint::series_data::QuerierVariant;
using entrypoint::series_data::QuerierVariantPtr;

extern "C" void prompp_series_data_data_storage_ctor(void* res) {
  using Result = struct {
    DataStoragePtr data_storage;
  };

  new (res) Result{.data_storage = std::make_unique<DataStorage>()};
}

extern "C" void prompp_series_data_data_storage_reset(void* args) {
  struct Arguments {
    DataStoragePtr data_storage;
  };

  static_cast<Arguments*>(args)->data_storage->reset();
}

extern "C" void prompp_series_data_data_storage_time_interval(void* args, void* res) {
  struct Arguments {
    DataStoragePtr data_storage;
  };
  struct Result {
    PromPP::Primitives::TimeInterval interval;
  };

  new (res) Result{.interval = series_data::Decoder::get_time_interval(*static_cast<Arguments*>(args)->data_storage)};
}

extern "C" void prompp_series_data_data_storage_queried_series_bitset_size(void* args, void* res) {
  struct Arguments {
    DataStoragePtr data_storage;
  };
  struct Result {
    uint32_t size;
  };

  new (res) Result{.size = static_cast<Arguments*>(args)->data_storage->queried_series_bitmap.get_write_size()};
}

extern "C" void prompp_series_data_data_storage_queried_series_bitset(void* args, void* res) {
  struct Arguments {
    DataStoragePtr data_storage;
  };
  struct Result {
    Slice<char> queried_series;
  };

  BytesStream stream(&static_cast<Result*>(res)->queried_series);
  static_cast<Arguments*>(args)->data_storage->queried_series_bitmap.write_to(stream);
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

  const auto arena_guard = in->data_storage->thread_arena_guard();

  const auto result = in->data_storage->queried_series_bitmap.read_from(stream);
  if (!result) {
    in->data_storage->queried_series_bitmap.reset(0);
  }
  new (res) Result{.result = result};
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
  using entrypoint::series_data::RangeQuerierWithArgumentsWrapperV2;
  using series_data::querier::Querier;

  struct Arguments {
    DataStoragePtr data_storage;
    Query query;
    PromPP::Primitives::Timestamp downsampling_ms;
    entrypoint::series_data::GoSelectHints* hints;
  };

  struct Result {
    QuerierVariantPtr querier{};
    QueryStatus status{};
    entrypoint::series_data::SerializedDataPtr* serialized_data{};
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = static_cast<Result*>(res);

  RangeQuerierWithArgumentsWrapperV2 querier(*in->data_storage, in->query, in->hints ? *in->hints : entrypoint::series_data::GoSelectHints{},
                                             out->serialized_data, in->downsampling_ms);
  querier.query();

  if (querier.need_loading()) {
    out->querier = std::make_unique<QuerierVariant>(std::in_place_index<1>, std::move(querier));
    out->status = QueryStatus::kNeedDataLoad;
  } else {
    out->status = QueryStatus::kSuccess;
  }
}

extern "C" void prompp_series_data_serialized_data_next(void* args, void* res) {
  struct Arguments {
    entrypoint::series_data::SerializedDataPtr serialized_data;
  };

  using Result = struct {
    uint32_t series_id;
    uint32_t chunk_ref;
  };
  const auto out = new (res) Result{};
  std::tie(out->series_id, out->chunk_ref) = static_cast<Arguments*>(args)->serialized_data->next();
}

extern "C" void prompp_series_data_serialized_data_dtor(void* args) {
  struct Arguments {
    entrypoint::series_data::SerializedDataPtr serialized_data;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_series_data_data_storage_instant_query(void* args, void* res) {
  using entrypoint::series_data::InstantQuerierWithArgumentsWrapperEntrypoint;
  using PromPP::Primitives::Timestamp;
  using series_data::InstantQuerier;

  struct Arguments {
    DataStoragePtr data_storage;
    SliceView<LabelSetID> label_set_ids;
    Timestamp timestamp;
    entrypoint::series_data::SampleWithGoLabels* samples;
  };

  using Result = struct {
    QuerierVariantPtr querier;
    QueryStatus status;
  };

  const auto in = static_cast<Arguments*>(args);

  auto samples = std::span(in->samples, in->label_set_ids.size());
  InstantQuerierWithArgumentsWrapperEntrypoint instant_querier(*in->data_storage, in->label_set_ids, in->timestamp, samples);
  instant_querier.query();

  if (instant_querier.need_loading()) {
    new (res) Result{
        .querier = std::make_unique<QuerierVariant>(std::in_place_type<InstantQuerierWithArgumentsWrapperEntrypoint>, std::move(instant_querier)),
        .status = QueryStatus::kNeedDataLoad,
    };
  } else {
    new (res) Result{.querier = nullptr, .status = QueryStatus::kSuccess};
  }
}

extern "C" void prompp_series_data_data_storage_query_final(void* args) {
  using entrypoint::series_data::QuerierVariantPtr;

  struct Arguments {
    Slice<QuerierVariantPtr> queriers;
  };

  const auto in = static_cast<Arguments*>(args);
  for (auto& querier_ptr : in->queriers) {
    std::visit([](auto& querier) { querier.query_finalize(); }, *querier_ptr);
    querier_ptr.reset();
  }
}

extern "C" void prompp_series_data_data_storage_query_first_timestamps(void* args, void* res) {
  using PromPP::Primitives::Timestamp;
  using series_data::Decoder;

  struct Arguments {
    DataStoragePtr data_storage;
    SliceView<LabelSetID> series_ids;
  };

  struct Result {
    Slice<Timestamp> timestamps;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = static_cast<Result*>(res);

  assert(in->series_ids.size() == out->timestamps.size());
  const auto& storage = *in->data_storage;

  std::ranges::transform(in->series_ids, out->timestamps.begin(),
                         [&storage](uint32_t series_id) { return Decoder::get_series_min_timestamp(storage, series_id); });
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

  out->allocated_memory = in->data_storage->allocated_memory();
}

extern "C" void prompp_series_data_data_storage_dtor(void* args) {
  struct Arguments {
    DataStoragePtr data_storage;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_series_data_chunk_recoder_ctor(void* args, void* res) {
  struct Arguments {
    entrypoint::head::LssVariantPtr lss;
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
      .chunk_recoder = std::make_unique<ChunkRecoderVariant>(
          std::in_place_type<ChunkRecoder>,
          ChunkRecoderIterator{ls_id_set.begin(), ls_id_set.end(), in->ls_id_batch_size, in->data_storage.get(), in->time_interval}, in->time_interval,
          in->downsampling_ms),
  };
}

extern "C" void prompp_series_data_serialized_chunk_recoder_ctor(void* args, void* res) {
  struct Arguments {
    entrypoint::series_data::SerializedDataPtr* serialized_data;
    PromPP::Primitives::TimeInterval time_interval;
  };
  struct Result {
    ChunkRecoderVariantPtr chunk_recoder;
  };

  const auto in = static_cast<Arguments*>(args);
  new (res) Result{
      .chunk_recoder = std::make_unique<ChunkRecoderVariant>(
          std::in_place_type<SerializedChunkRecoder>,
          series_data::chunk::SerializedChunkIterator{in->serialized_data->get()->get_buffer_view(), in->serialized_data->get()->get_chunks_view()},
          in->time_interval, series_data::decoder::decorator::kNoDownsampling),
  };
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
  explicit Unloader(DataStorage& storage) : unloader(storage) {}

  series_data::unloading::Unloader unloader;
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

  new (res) Result{.unloader = std::make_unique<Unloader>(*static_cast<Arguments*>(args)->data_storage)};
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
  const auto out = new (res) Result{.loader = std::make_unique<LoaderVariant>(std::in_place_type<Loader>, *in->data_storage)};
  auto& loader = std::get<Loader>(*out->loader);

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
    entrypoint::head::LssVariantPtr lss;
    uint32_t ls_id_batch_size;
    DataStoragePtr data_storage;
  };

  struct Result {
    LoaderVariantPtr loader;
  };

  const auto in = static_cast<Arguments*>(args);
  auto& ls_id_set = std::get<QueryableEncodingBimap>(*in->lss).ls_id_set();
  new (res) Result{
      .loader =
          std::make_unique<LoaderVariant>(std::in_place_type<RevertableLoader>, *in->data_storage, ls_id_set.begin(), ls_id_set.end(), in->ls_id_batch_size),
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