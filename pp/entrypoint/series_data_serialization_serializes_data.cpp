#include "series_data_serialization_serializes_data.h"

#include "head/serialization.h"

extern "C" void prompp_series_data_serialization_serialized_data_next(void* args, void* res) {
  struct Arguments {
    entrypoint::head::SerializedDataPtr serialized_data;
  };

  using Result = struct {
    uint32_t series_id;
  };

  new (res) Result{.series_id = reinterpret_cast<Arguments*>(args)->serialized_data->next_series()};
}

extern "C" void prompp_series_data_serialization_serialized_data_iterator(void* args, void* res) {
  struct Arguments {
    entrypoint::head::SerializedDataPtr serialized_data;
  };

  using Result = struct {
    entrypoint::head::SerializedDataIteratorPtr iterator;
  };

  new (res) Result{.iterator = std::make_unique<series_data::serialization::SerializedData::SerializedSeriesIterator>(
                       static_cast<Arguments*>(args)->serialized_data->create_current_series_iterator())};
}

extern "C" void prompp_series_data_serialization_serialized_data_iterator_next(void* args, void* res) {
  using series_data::decoder::DecodeIteratorSentinel;

  struct Arguments {
    entrypoint::head::SerializedDataIteratorPtr iterator;
  };

  using Result = struct {
    int64_t timestamp{};
    double value{};
    bool has_value;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);

  if (*in->iterator == DecodeIteratorSentinel{}) {
    new (res) Result{.has_value = false};
  } else {
    const auto sample = **(in->iterator);
    new (res) Result{.timestamp = sample.timestamp, .value = sample.value, .has_value = true};
    ++(*in->iterator);
  }
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