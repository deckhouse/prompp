#include "series_data_serialization_serialized_data.h"

#include "series_data/serialization.h"

extern "C" void prompp_series_data_serialization_serialized_data_next(void* args, void* res) {
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

extern "C" void prompp_series_data_serialization_serialized_data_iterator_ctor(void* args) {
  struct Arguments {
    entrypoint::series_data::SerializedDataIterator* iterator;
    entrypoint::series_data::SerializedDataPtr serialized_data;
    uint32_t chunk_ref;
  };

  const auto in = static_cast<Arguments*>(args);
  new (in->iterator) entrypoint::series_data::SerializedDataIterator(in->serialized_data->iterator(in->chunk_ref));
}

extern "C" void prompp_series_data_serialization_serialized_data_iterator_next(void* iterator) {
  ++(*static_cast<entrypoint::series_data::SerializedDataIterator*>(iterator));
}

extern "C" void prompp_series_data_serialization_serialized_data_iterator_seek(void* args) {
  using series_data::decoder::DecodeIteratorSentinel;

  struct Arguments {
    entrypoint::series_data::SerializedDataIterator* iterator;
    int64_t target_timestamp;
  };

  for (const Arguments* in = static_cast<Arguments*>(args); *in->iterator != DecodeIteratorSentinel{}; ++(*in->iterator)) {
    if ((*in->iterator)->timestamp >= in->target_timestamp) {
      return;
    }
  }
}

extern "C" void prompp_series_data_serialization_serialized_data_iterator_reset(void* args) {
  struct Arguments {
    entrypoint::series_data::SerializedDataIterator* iterator;
    entrypoint::series_data::SerializedDataPtr serialized_data;
    uint32_t chunk_ref;
  };

  const Arguments* in = static_cast<Arguments*>(args);
  in->iterator->reset(in->serialized_data->get_buffer_view(), in->serialized_data->get_chunks_view(), in->chunk_ref);
}

extern "C" void prompp_series_data_serialization_serialized_data_multi_series_iterator_ctor(void* args) {
  struct Arguments {
    entrypoint::series_data::MultiSeriesDecodeIterator* iterator;
    entrypoint::series_data::SerializedDataPtr serialized_data;
    PromPP::Primitives::Go::SliceView<uint32_t> series_ids;
  };

  const auto in = static_cast<Arguments*>(args);
  new (in->iterator) entrypoint::series_data::MultiSeriesDecodeIterator(in->serialized_data->multi_series_iterator(in->series_ids.span()));
}

extern "C" void prompp_series_data_serialization_serialized_data_multi_series_iterator_next(void* iterator) {
  ++(*static_cast<entrypoint::series_data::MultiSeriesDecodeIterator*>(iterator));
}

extern "C" void prompp_series_data_serialization_serialized_data_multi_series_iterator_dtor(void* iterator) {
  static_cast<entrypoint::series_data::MultiSeriesDecodeIterator*>(iterator)->~MultiSeriesDecodeIterator();
}

extern "C" void prompp_series_data_serialization_serialized_data_dtor(void* args) {
  struct Arguments {
    entrypoint::series_data::SerializedDataPtr serialized_data;
  };

  static_cast<Arguments*>(args)->~Arguments();
}