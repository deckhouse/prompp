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

extern "C" void prompp_series_data_serialization_serialized_data_iterator_ctor(void* args, void* res) {
  struct Arguments {
    entrypoint::head::SerializedDataPtr serialized_data;
    uint32_t chunk_ref;
  };

  using Result = struct {
    entrypoint::head::SerializedDataIteratorPtr iterator;
  };

  new (res) Result{.iterator = std::make_unique<series_data::serialization::SerializedDataView::SeriesIterator>(
                       static_cast<Arguments*>(args)->serialized_data->iterator(static_cast<Arguments*>(args)->chunk_ref))};
}

extern "C" void prompp_series_data_serialization_serialized_data_iterator_next(void* args, void* res) {
  using series_data::decoder::DecodeIteratorSentinel;

  struct Arguments {
    entrypoint::head::SerializedDataIteratorPtr iterator;
  };

  struct Result {
    int64_t timestamp{};
    double value{};
    bool has_value;
  };

  const Arguments* in = static_cast<Arguments*>(args);

  if (*in->iterator == DecodeIteratorSentinel{}) {
    new (res) Result{.has_value = false};
  } else {
    const auto sample = **(in->iterator);
    new (res) Result{.timestamp = sample.timestamp, .value = sample.value, .has_value = true};
    ++(*in->iterator);
  }
}

extern "C" void prompp_series_data_serialization_serialized_data_iterator_seek(void* args, void* res) {
  using series_data::decoder::DecodeIteratorSentinel;

  struct Arguments {
    entrypoint::head::SerializedDataIteratorPtr iterator;
    int64_t target_timestamp;
  };

  struct Result {
    int64_t timestamp{};
    double value{};
    bool has_value;
  };

  const Arguments* in = static_cast<Arguments*>(args);
  const auto out = static_cast<Result*>(res);

  while (true) {
    if (*in->iterator == DecodeIteratorSentinel{}) {
      out->has_value = false;
      return;
    }

    const auto sample = **(in->iterator);
    if (sample.timestamp < in->target_timestamp) {
      ++(*in->iterator);
      continue;
    }

    out->timestamp = sample.timestamp;
    out->value = sample.value;
    out->has_value = true;
    ++(*in->iterator);
    return;
  }
}

extern "C" void prompp_series_data_serialization_serialized_data_iterator_reset(void* args) {
  struct Arguments {
    entrypoint::head::SerializedDataPtr serialized_data;
    entrypoint::head::SerializedDataIteratorPtr iterator;
    uint32_t chunk_ref;
  };

  const Arguments* in = static_cast<Arguments*>(args);
  in->iterator->reset(in->serialized_data->get_buffer_view(), in->serialized_data->get_chunks_view(), in->chunk_ref);
}

extern "C" void prompp_series_data_serialization_serialized_data_iterator_dtor(void* args) {
  struct Arguments {
    entrypoint::head::SerializedDataIteratorPtr iterator;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_series_data_serialization_serialized_data_dtor(void* args) {
  struct Arguments {
    entrypoint::head::SerializedDataPtr serialized_data;
  };

  static_cast<Arguments*>(args)->~Arguments();
}