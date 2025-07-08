#include "series_data_data_storage.h"

#include "head/chunk_recoder.h"
#include "head/data_storage.h"
#include "head/lss.h"
#include "primitives/go_slice.h"
#include "series_data/data_storage.h"
#include "series_data/querier/instant_querier.h"
#include "series_data/querier/querier.h"
#include "series_data/serialization/serializer.h"
#include "series_data/unloading/loader.h"
#include "series_data/unloading/unloader.h"

using entrypoint::head::DataStoragePtr;
using entrypoint::head::QueryableEncodingBimap;
using ChunkRecoderIterator = head::ChunkRecoderIterator<QueryableEncodingBimap::LsIdSet::const_iterator, QueryableEncodingBimap::LsIdSet::const_iterator>;
using ChunkRecoder = head::ChunkRecoder<ChunkRecoderIterator>;

using SerializedChunkRecoder = head::ChunkRecoder<series_data::chunk::SerializedChunkIterator>;

using ChunkRecoderVariant = std::variant<ChunkRecoder, SerializedChunkRecoder>;
using ChunkRecoderVariantPtr = std::unique_ptr<ChunkRecoderVariant>;

using LoaderPtr = std::unique_ptr<series_data::unloading::Loader>;

extern "C" void prompp_series_data_data_storage_ctor(void* res) {
  using Result = struct {
    DataStoragePtr data_storage;
  };

  new (res) Result{.data_storage = std::make_unique<series_data::DataStorage>()};
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

extern "C" void prompp_series_data_data_storage_query(void* args, void* res) {
  using PromPP::Primitives::LabelSetID;
  using PromPP::Primitives::Go::Slice;
  using series_data::DataStorage;
  using Query = series_data::querier::Query<Slice<LabelSetID>>;
  using PromPP::Primitives::Go::BytesStream;
  using series_data::querier::Querier;
  using series_data::serialization::Serializer;

  struct Arguments {
    DataStorage* data_storage;
    Query query;
  };

  using Result = struct {
    Slice<char> serialized_chunks;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = new (res) Result();

  Querier querier{*in->data_storage};
  const auto& queried_chunk_list = querier.query(in->query);

  if (querier.need_loading()) {
    series_data::unloading::Loader loader(*in->data_storage, querier);

    for (const auto& buffer : in->data_storage->unloaded_snapshots) {
      loader.load_next(buffer);
    }
    loader.load_finalize();
  }

  Serializer serializer{*in->data_storage};
  BytesStream bytes_stream{&out->serialized_chunks};
  serializer.serialize(queried_chunk_list, bytes_stream);
}

extern "C" void prompp_series_data_data_storage_instant_query(void* args) {
  using PromPP::Primitives::LabelSetID;
  using PromPP::Primitives::Timestamp;
  using PromPP::Primitives::Go::SliceView;
  using series_data::DataStorage;
  using series_data::InstantQuerier;
  using series_data::encoder::Sample;

  struct Arguments {
    DataStorage* data_storage;
    SliceView<LabelSetID> label_set_ids;
    Timestamp timestamp;
    SliceView<Sample> samples;
  };

  const auto in = static_cast<Arguments*>(args);

  InstantQuerier instant_querier{*in->data_storage};
  instant_querier.query(in->samples, in->label_set_ids, in->timestamp);

  if (instant_querier.need_loading()) {
    series_data::unloading::Loader loader(*in->data_storage, instant_querier);

    for (const auto& buffer : in->data_storage->unloaded_snapshots) {
      loader.load_next(buffer);
    }
    loader.load_finalize();

    instant_querier.query_reload(in->samples, in->label_set_ids, in->timestamp);
  }
}

extern "C" void prompp_series_data_data_storage_allocated_memory(void* args, void* res) {
  using series_data::DataStorage;

  struct Arguments {
    DataStorage* data_storage;
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
    DataStoragePtr data_storage;
    PromPP::Primitives::TimeInterval time_interval;
  };
  struct Result {
    ChunkRecoderVariantPtr chunk_recoder;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto& ls_id_set = std::get<QueryableEncodingBimap>(*in->lss).ls_id_set();

  if (in->data_storage->unloaded_series_bitmap.popcount() != 0) {
    series_data::unloading::Loader loader{*in->data_storage};
    for (const auto& buffer : in->data_storage->unloaded_snapshots) {
      loader.load_next(buffer);
    }
    loader.load_finalize();
  }

  new (res) Result{
      .chunk_recoder = std::make_unique<ChunkRecoderVariant>(
          std::in_place_type<ChunkRecoder>, ChunkRecoderIterator{ls_id_set.begin(), ls_id_set.end(), in->data_storage.get(), in->time_interval},
          in->time_interval),
  };
}

extern "C" void prompp_series_data_serialized_chunk_recoder_ctor(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<uint8_t> buffer;
    PromPP::Primitives::TimeInterval time_interval;
  };
  struct Result {
    ChunkRecoderVariantPtr chunk_recoder;
  };

  const auto in = static_cast<Arguments*>(args);
  new (res) Result{
      .chunk_recoder = std::make_unique<ChunkRecoderVariant>(std::in_place_type<SerializedChunkRecoder>,
                                                             series_data::chunk::SerializedChunkIterator{in->buffer.span()}, in->time_interval),
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
    PromPP::Primitives::Go::SliceView<const uint8_t> buffer;
  };

  const auto in = static_cast<const Arguments*>(args);
  const auto out = static_cast<Result*>(res);
  std::visit(
      [out](auto& chunk_recoder) PROMPP_LAMBDA_INLINE {
        chunk_recoder.recode_next_chunk(*out);
        out->has_more_data = chunk_recoder.has_more_data();
        out->buffer.reset_to(chunk_recoder.bytes());
      },
      *in->chunk_recoder);
}

extern "C" void prompp_series_data_chunk_recoder_dtor(void* args) {
  struct Arguments {
    ChunkRecoderVariantPtr chunk_recoder;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_series_data_data_storage_unload(void* args, void* res) {
  using PromPP::Primitives::LabelSetID;
  using PromPP::Primitives::Go::BytesStream;
  using PromPP::Primitives::Go::Slice;
  using series_data::DataStorage;
  using series_data::unloading::Unloader;

  struct Arguments {
    DataStorage* data_storage;
  };

  using Result = struct {
    Slice<char> unloaded_data;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = new (res) Result();

  Unloader unloader{*in->data_storage};
  BytesStream bytes_stream{&out->unloaded_data};
  unloader.unload(bytes_stream);

  in->data_storage->unloaded_snapshots.emplace_back(reinterpret_cast<const uint8_t*>(out->unloaded_data.data()),
                                                    reinterpret_cast<const uint8_t*>(out->unloaded_data.data()) + out->unloaded_data.size());
}

extern "C" void prompp_series_data_data_storage_loader_ctor(void* args, void* res) {
  using PromPP::Primitives::LabelSetID;
  using PromPP::Primitives::Go::SliceView;
  using series_data::DataStorage;
  using series_data::unloading::Loader;

  struct Arguments {
    DataStorage* data_storage;
    SliceView<LabelSetID> label_sets;
  };

  struct Result {
    LoaderPtr loader;
  };

  const auto in = static_cast<Arguments*>(args);

  new (res) Result{.loader = std::make_unique<Loader>(*(in->data_storage), in->label_sets)};
}

extern "C" void prompp_series_data_data_storage_loader_load_next(void* args) {
  using PromPP::Primitives::Go::SliceView;

  struct Arguments {
    LoaderPtr loader;
    SliceView<const uint8_t> buffer;
  };

  const auto in = static_cast<Arguments*>(args);

  in->loader->load_next(in->buffer.span());
}

extern "C" void prompp_series_data_data_storage_loader_load_finalize(void* args) {
  struct Arguments {
    LoaderPtr loader;
  };

  static_cast<Arguments*>(args)->loader->load_finalize();
}

extern "C" void prompp_series_data_data_storage_loader_dtor(void* args) {
  struct Arguments {
    LoaderPtr loader;
  };

  static_cast<Arguments*>(args)->~Arguments();
}