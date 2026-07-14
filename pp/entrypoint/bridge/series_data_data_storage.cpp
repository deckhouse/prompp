#include "series_data_data_storage.h"
#include "annotations.h"

#include <cassert>
#include <spanstream>

#include "entrypoint/types/data_storage.h"
#include "entrypoint/types/loader.h"
#include "entrypoint/types/lss.h"
#include "entrypoint/types/querier.h"
#include "entrypoint/types/serialized_data.h"
#include "head/chunk_recoder.h"
#include "primitives/go_slice.h"
#include "series_data/data_storage.h"
#include "series_data/decoder.h"
#include "series_data/querier/instant_querier.h"
#include "series_data/querier/querier.h"
#include "series_data/unloading/loader.h"
#include "series_data/unloading/unloader.h"

namespace {

using entrypoint::types::DataStoragePtr;
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

using LoaderVariant = std::variant<series_data::unloading::Loader, RevertableLoader>;
using LoaderVariantPtr = std::unique_ptr<LoaderVariant>;
static_assert(sizeof(LoaderVariantPtr) == sizeof(void*));

using entrypoint::types::QuerierType;
using entrypoint::types::QuerierVariant;
using entrypoint::types::QuerierVariantPtr;

struct Unloader {
  explicit Unloader(DataStorage& storage) : unloader(storage) {}

  series_data::unloading::Unloader unloader;
  Slice<char> snapshot;
};

using UnloaderPtr = std::unique_ptr<Unloader>;
static_assert(sizeof(UnloaderPtr) == sizeof(void*));

}  // namespace

/**
 * @brief Construct a new series data DataStorage
 *
 * @param res {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_ctor(void* res) {
  using Result = struct {
    DataStoragePtr data_storage;
  };

  new (res) Result{.data_storage = std::make_unique<DataStorage>()};
}

/**
 * @brief Resets DataStorage to initial state
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_reset(void* args) {
  struct Arguments {
    DataStoragePtr data_storage;
  };

  static_cast<Arguments*>(args)->data_storage->reset();
}

/**
 * @brief Get min max timestamps in storage
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 *
 * @param res {
 *     interval struct {
 *        min int64
 *        max int64
 *     }
 * }
 *
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_time_interval(void* args, void* res) {
  struct Arguments {
    DataStoragePtr data_storage;
  };
  struct Result {
    PromPP::Primitives::TimeInterval interval;
  };

  new (res) Result{.interval = series_data::Decoder::get_time_interval(*static_cast<Arguments*>(args)->data_storage)};
}

/**
 * @brief Get queried series bitset memory size
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 *
 * @param res {
 *     size uint32 // queried series bitset memory size
 * }
 *
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_queried_series_bitset_size(void* args, void* res) {
  struct Arguments {
    DataStoragePtr data_storage;
  };
  struct Result {
    uint32_t size;
  };

  new (res) Result{.size = static_cast<Arguments*>(args)->data_storage->queried_series_bitmap.get_write_size()};
}

/**
 * @brief Get queried series bitset memory
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 *
 * @param res {
 *     queriedSeries []byte // queried series bitset (memory allocated in c++)
 * }
 *
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_queried_series_bitset(void* args, void* res) {
  struct Arguments {
    DataStoragePtr data_storage;
  };
  struct Result {
    Slice<char> queried_series;
  };

  BytesStream stream(&static_cast<Result*>(res)->queried_series);
  static_cast<Arguments*>(args)->data_storage->queried_series_bitmap.write_to(stream);
}

/**
 * @brief Get queried series bitset memory
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 *     queriedSeries []byte // queried series bitset memory
 * }
 *
 * @param res {
 *     result bool // load result
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_queried_series_set_bitset(void* args, void* res) {
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

/**
 * @brief Queries data storage and serializes result (new serialization model).
 *
 * @param args {
 *     dataStorage    uintptr          // pointer to constructed data storage
 *     query          DataStorageQuery // query
 * }
 *
 * @param res {
 *     Querier uintptr        // pointer to constructed Querier if data loading is needed.
 *                            // If constructed (!= 0) it must be destroyed by calling prompp_series_data_data_storage_query_final.
 *     Status  uint8          // status of a query (0 - Success, 1 - Data loading is needed)
 *     serializedData uintptr // pointer to serialized data
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_query_v2(void* args, void* res) {
  using Query = series_data::querier::Query<Slice<LabelSetID>>;
  using entrypoint::types::RangeQuerierWithArgumentsWrapperV2;
  using series_data::querier::Querier;

  struct Arguments {
    DataStoragePtr data_storage;
    Query query;
  };

  struct Result {
    QuerierVariantPtr querier{};
    QueryStatus status{};
    entrypoint::types::SerializedDataPtr* serialized_data{};
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = static_cast<Result*>(res);

  RangeQuerierWithArgumentsWrapperV2 querier(*in->data_storage, in->query, out->serialized_data);
  querier.query();

  if (querier.need_loading()) {
    out->querier = std::make_unique<QuerierVariant>(std::in_place_index<1>, std::move(querier));
    out->status = QueryStatus::kNeedDataLoad;
  } else {
    out->status = QueryStatus::kSuccess;
  }
}

/**
 * @brief return instant series at given timestamp for label sets.
 *
 * @param args {
 *        dataStorage uintptr      // pointer to constructed data storage
 *        labelSetIDs []uint32     // series ids
 *        timestamp   int64        // timestamp
 *        samples     uintptr      // pointer to samples data
 * }
 * @param res {
 *     InstantQuerier uintptr // pointer to constructed Querier if data loading is needed
 *     Status uint8           // status of a query (0 - Success, 1 - Data loading is needed)
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_instant_query(void* args, void* res) {
  using entrypoint::types::InstantQuerierWithArgumentsWrapperEntrypoint;
  using PromPP::Primitives::Timestamp;
  using series_data::InstantQuerier;

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

/**
 * @brief finishes all Queriers after data load.
 *
 * @param args {
 *        queriers []uintptr    // slice of pointers to Queriers
 *        }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_query_final(void* args) {
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

/**
 * @brief Get the first sample timestamp per series
 *
 * @param args {
 *        dataStorage uintptr  // pointer to constructed data storage
 *        seriesIds   []uint32 // series ids
 * }
 * @param res {
 *        timestamps []int64  // same length as seriesIds; filled from storage
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_query_first_timestamps(void* args, void* res) {
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

/**
 * @brief Queries data storage and serializes result.
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 *
 * @param res {
 *     allocated_memory uint64 // serialized data
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_allocated_memory(void* args, void* res) {
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

/**
 * @brief series data DataStorage destructor.
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_dtor(void* args) {
  struct Arguments {
    DataStoragePtr data_storage;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

/**
 * @brief Construct a new ChunkRecoder object for recode all non-empty chunks in dataStorage
 *
 * @param args {
 *     lss uintptr            // pointer to constructed label sets
 *     lsIdBatchSize uint32   // size of ls batch for recoding
 *     dataStorage   uintptr  // pointer to constructed data storage
 *     time_interval struct { closed interval [min, max]
 *        min int64
 *        max int64
 *     }
 * }
 * @param res {
 *     chunk_recoder uintptr // pointer to chunk recoder
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_chunk_recoder_ctor(void* args, void* res) {
  struct Arguments {
    entrypoint::types::LssVariantPtr lss;
    uint32_t ls_id_batch_size;
    DataStoragePtr data_storage;
    PromPP::Primitives::TimeInterval time_interval;
  };
  struct Result {
    ChunkRecoderVariantPtr chunk_recoder;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto& ls_id_set = std::get<QueryableEncodingBimap>(*in->lss).ls_id_set();

  new (res) Result{
      .chunk_recoder = std::make_unique<ChunkRecoderVariant>(
          std::in_place_type<ChunkRecoder>,
          ChunkRecoderIterator{ls_id_set.begin(), ls_id_set.end(), in->ls_id_batch_size, in->data_storage.get(), in->time_interval}, in->time_interval),
  };
}

/**
 * @brief Construct a new ChunkRecoder object to recode all serialized chunks (new model)
 *
 * @param args {
 *     serializedData *uintptr // pointer to serialized data
 *     time_interval struct { // closed interval [min, max]
 *        min int64
 *        max int64
 *     }
 * }
 * @param res {
 *     chunk_recoder uintptr // pointer to chunk recoder
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_serialized_chunk_recoder_ctor(void* args, void* res) {
  struct Arguments {
    entrypoint::types::SerializedDataPtr* serialized_data;
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
          in->time_interval),
  };
}

/**
 * @brief Get chunk encoded in prometheus format
 *
 * @param args {
 *     chunk_recoder uintptr // pointer to chunk recoder
 * }
 * @param res {
 *     interval struct {
 *        min int64
 *        max int64
 *     }
 *     series_id     uint32
 *     samples_count uint8
 *     has_more_data bool
 *     data          []byte // SliceView to recoded chunk data
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_chunk_recoder_recode_next_chunk(void* args, void* res) {
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

/**
 * @brief Advance ChunkRecoder::ls_id_iterator to next batch
 *
 * @param args {
 *     chunk_recoder uintptr // pointer to chunk recoder
 * }
 *
 * @param res {
 *     hasMoreData bool  // true if chunk recoder has more
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_chunk_recoder_next_batch(void* args, void* res) {
  struct Arguments {
    ChunkRecoderVariantPtr chunk_recoder;
  };
  struct Result {
    bool has_more_data;
  };

  auto& recoder = std::get<ChunkRecoder>(*static_cast<const Arguments*>(args)->chunk_recoder);
  new (res) Result{.has_more_data = recoder.chunk_iterator().next_batch()};
}

/**
 * @brief Destruct ChunkRecoder object
 *
 * @param args {
 *     chunk_recoder  uintptr  // pointer to chunk recoder
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_chunk_recoder_dtor(void* args) {
  struct Arguments {
    ChunkRecoderVariantPtr chunk_recoder;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

/**
 * @brief Construct unloader
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 * }
 *
 * @param res {
 *     unloader uintptr // pointer to constructed unloader
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_unloader_ctor(void* args, void* res) {
  struct Arguments {
    DataStoragePtr data_storage;
  };

  struct Result {
    UnloaderPtr unloader;
  };

  new (res) Result{.unloader = std::make_unique<Unloader>(*static_cast<Arguments*>(args)->data_storage)};
}

/**
 * @brief Destruct unloader
 *
 * @param args {
 *     unloader uintptr // pointer to constructed unloader
 * }
 *
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_unloader_dtor(void* args) {
  struct Arguments {
    UnloaderPtr unloader;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

/**
 * @brief Create data snapshot of unused series
 *
 * @param args {
 *     unloader uintptr // pointer to constructed unloader
 * }
 *
 * @param res {
 *     unloadedData []byte // encoded unload data
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_unloader_create_snapshot(void* args, void* res) {
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

/**
 * @brief Unload data from DataStorage
 *
 * @param args {
 *     unloader uintptr // pointer to constructed unloader
 * }
 *
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_unloader_unload(void* args) {
  struct Arguments {
    UnloaderPtr unloader;
  };

  auto& unloader = static_cast<const Arguments*>(args)->unloader->unloader;
  const auto arena_guard = unloader.storage().thread_arena_guard();

  unloader.unload();
}

/**
 * @brief Construct Loader to load previously unqueried series
 *
 * @param args {
 *     dataStorage uintptr // pointer to constructed data storage
 *     labelSetIDs []uint32 // label sets from rejected query
 * }
 *
 *  @param res {
 *     loader uintptr // pointer to loader
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_loader_ctor(void* args, void* res) {
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

/**
 * @brief Construct RevertableLoader to load previously unqueried series
 *
 * @param args {
 *     lss uintptr            // pointer to constructed label sets
 *     lsIdBatchSize uint32   // size of ls batch for recoding
 *     dataStorage   uintptr  // pointer to constructed data storage
 * }
 *
 *  @param res {
 *     loader uintptr // pointer to loader
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_revertable_loader_ctor(void* args, void* res) {
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
      .loader =
          std::make_unique<LoaderVariant>(std::in_place_type<RevertableLoader>, *in->data_storage, ls_id_set.begin(), ls_id_set.end(), in->ls_id_batch_size),
  };
}

/**
 * @brief Loads next previously unloaded snapshot of data
 *
 * @param args {
 *     loader uintptr // pointer to loader
 *     buffer []byte // SliceView to unloaded snapshot
 *     is_final bool // flag if this buffer corresponds to the last snapshot
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_loader_load_next(void* args) {
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

/**
 * @brief Advance RevertableLoader::iterator to next batch
 *
 * @param args {
 *     loader uintptr // pointer to loader
 * }
 *
 * @param res {
 *     hasMoreData bool  // true if chunk recoder has more
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_revertable_loader_next_batch(void* args, void* res) {
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

/**
 * @brief Destroy Loader object
 *
 * @param args {
 *     loader uintptr // pointer to loader
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_data_storage_loader_dtor(void* args) {
  struct Arguments {
    LoaderVariantPtr loader;
  };

  static_cast<Arguments*>(args)->~Arguments();
}
