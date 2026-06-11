#include "series_data_serialization_serialized_data.h"

#include "head/serialization.h"

extern "C" void prompp_series_data_serialization_serialized_data_next(void* args, void* res) {
  struct Arguments {
    entrypoint::head::SerializedDataPtr serialized_data;
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
    entrypoint::head::SerializedDataIterator* iterator;
    entrypoint::head::SerializedDataPtr serialized_data;
    uint32_t chunk_ref;
  };

  const auto in = static_cast<Arguments*>(args);
  new (in->iterator) entrypoint::head::SerializedDataIterator(in->serialized_data->iterator(in->chunk_ref));
}

extern "C" void prompp_series_data_serialization_serialized_data_iterator_next(void* iterator) {
  using series_data::decoder::DecodeIteratorSentinel;

  ++(*static_cast<entrypoint::head::SerializedDataIterator*>(iterator));
}

extern "C" void prompp_series_data_serialization_serialized_data_iterator_seek(void* args) {
  using series_data::decoder::DecodeIteratorSentinel;

  struct Arguments {
    entrypoint::head::SerializedDataIterator* iterator;
    int64_t target_timestamp;
  };

  const Arguments* in = static_cast<Arguments*>(args);
  in->iterator->seek_to(in->target_timestamp);
}

extern "C" void prompp_series_data_serialization_serialized_data_iterator_reset(void* args) {
  struct Arguments {
    entrypoint::head::SerializedDataIterator* iterator;
    entrypoint::head::SerializedDataPtr serialized_data;
    uint32_t chunk_ref;
  };

  const Arguments* in = static_cast<Arguments*>(args);
  in->iterator->reset(in->serialized_data->get_buffer_view(), in->serialized_data->get_chunks_view(), in->chunk_ref);
}

extern "C" void prompp_series_data_serialization_serialized_data_dtor(void* args) {
  struct Arguments {
    entrypoint::head::SerializedDataPtr serialized_data;
  };

  static_cast<Arguments*>(args)->~Arguments();
}