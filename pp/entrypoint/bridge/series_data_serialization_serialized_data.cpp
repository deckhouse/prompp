#include "series_data_serialization_serialized_data.h"

#include <variant>

#include "entrypoint/types/serialized_data.h"

extern "C" void prompp_series_data_serialization_serialized_data_next(void* args, void* res) {
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

extern "C" void prompp_series_data_serialization_serialized_data_iterator_ctor(void* args) {
  struct Arguments {
    entrypoint::types::SerializedDataIterator* iterator;
    entrypoint::types::SerializedDataPtr serialized_data;
    uint32_t chunk_ref;
  };

  const auto in = static_cast<Arguments*>(args);
  std::visit([in](const auto& serialized_data) { new (in->iterator) entrypoint::types::SerializedDataIterator(serialized_data.iterator(in->chunk_ref)); },
             *in->serialized_data);
}

extern "C" void prompp_series_data_serialization_serialized_data_iterator_next(void* iterator) {
  using series_data::decoder::DecodeIteratorSentinel;

  ++(*static_cast<entrypoint::types::SerializedDataIterator*>(iterator));
}

extern "C" void prompp_series_data_serialization_serialized_data_iterator_seek(void* args) {
  using series_data::decoder::DecodeIteratorSentinel;

  struct Arguments {
    entrypoint::types::SerializedDataIterator* iterator;
    int64_t target_timestamp;
  };

  const Arguments* in = static_cast<Arguments*>(args);
  in->iterator->seek_to(in->target_timestamp);
}

extern "C" void prompp_series_data_serialization_serialized_data_iterator_reset(void* args) {
  struct Arguments {
    entrypoint::types::SerializedDataIterator* iterator;
    entrypoint::types::SerializedDataPtr serialized_data;
    uint32_t chunk_ref;
  };

  const Arguments* in = static_cast<Arguments*>(args);
  std::visit([in](const auto& serialized_data) { in->iterator->reset(serialized_data.get_buffer_view(), serialized_data.get_chunks_view(), in->chunk_ref); },
             *in->serialized_data);
}

extern "C" void prompp_series_data_serialization_serialized_data_dtor(void* args) {
  struct Arguments {
    entrypoint::types::SerializedDataPtr serialized_data;
  };

  static_cast<Arguments*>(args)->~Arguments();
}