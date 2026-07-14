#include "multiseries_iterator.h"

#include "entrypoint/types/serialization.h"

extern "C" void prompp_series_data_serialization_serialized_data_multi_series_iterator_ctor(void* args) {
  struct Arguments {
    entrypoint::types::MultiSeriesDecodeIterator* iterator;
    entrypoint::types::SerializedDataPtr serialized_data;
    PromPP::Primitives::Go::SliceView<uint32_t> series_ids;
  };

  const auto in = static_cast<Arguments*>(args);
  in->serialized_data->construct_multi_series_iterator(in->iterator, in->series_ids.span());
}

extern "C" void prompp_series_data_serialization_serialized_data_multi_series_iterator_reset(void* args) {
  struct Arguments {
    entrypoint::types::MultiSeriesDecodeIterator* iterator;
    entrypoint::types::SerializedDataPtr serialized_data;
    PromPP::Primitives::Go::SliceView<uint32_t> series_ids;
  };

  const auto in = static_cast<Arguments*>(args);
  in->serialized_data->reset_multi_series_iterator(*in->iterator, in->series_ids.span());
}

extern "C" void prompp_series_data_serialization_serialized_data_multi_series_iterator_next(void* iterator) {
  ++(*static_cast<entrypoint::types::MultiSeriesDecodeIterator*>(iterator));
}

extern "C" void prompp_series_data_serialization_serialized_data_multi_series_iterator_dtor(void* iterator) {
  std::destroy_at(static_cast<entrypoint::types::MultiSeriesDecodeIterator*>(iterator));
}
