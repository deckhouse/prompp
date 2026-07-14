#include "aggregation_iterator.h"

#include "entrypoint/types/serialization.h"

extern "C" void prompp_series_data_serialization_serialized_data_aggregation_iterator_ctor(void* args) {
  struct Arguments {
    entrypoint::types::AggregationIterator* iterator;
    entrypoint::types::SerializedDataPtr serialized_data;
    uint32_t chunk_ref;
  };

  const auto in = static_cast<Arguments*>(args);
  std::construct_at(in->iterator, in->serialized_data->aggregation_iterator(in->chunk_ref));
}

extern "C" void prompp_series_data_serialization_serialized_data_aggregation_iterator_next(void* iterator) {
  using series_data::decoder::DecodeIteratorSentinel;

  ++(*static_cast<entrypoint::types::AggregationIterator*>(iterator));
}

extern "C" void prompp_series_data_serialization_serialized_data_aggregation_iterator_reset(void* args) {
  struct Arguments {
    entrypoint::types::AggregationIterator* iterator;
    entrypoint::types::SerializedDataPtr serialized_data;
    uint32_t chunk_ref;
  };

  const Arguments* in = static_cast<Arguments*>(args);
  *in->iterator = in->serialized_data->aggregation_iterator(in->chunk_ref);
}
