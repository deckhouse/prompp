#include "multiseries_iterator.h"

#include "series_data/serialization.h"

extern "C" void prompp_series_data_serialization_serialized_data_multi_series_iterator_ctor(void* args) {
  struct Arguments {
    entrypoint::series_data::MultiSeriesDecodeIterator* iterator;
    entrypoint::series_data::SerializedDataPtr serialized_data;
    PromPP::Primitives::Go::SliceView<uint32_t> series_ids;
  };

  const auto in = static_cast<Arguments*>(args);
  std::construct_at(in->iterator, in->serialized_data->multi_series_iterator(in->series_ids.span()));
}

extern "C" void prompp_series_data_serialization_serialized_data_multi_series_iterator_reset(void* args) {
  struct Arguments {
    entrypoint::series_data::MultiSeriesDecodeIterator* iterator;
    entrypoint::series_data::SerializedDataPtr serialized_data;
    PromPP::Primitives::Go::SliceView<uint32_t> series_ids;
  };

  const auto in = static_cast<Arguments*>(args);
  in->serialized_data->reset_multi_series_iterator(*in->iterator, in->series_ids.span());
}

extern "C" void prompp_series_data_serialization_serialized_data_multi_series_iterator_next(void* iterator) {
  ++(*static_cast<entrypoint::series_data::MultiSeriesDecodeIterator*>(iterator));
}

extern "C" void prompp_series_data_serialization_serialized_data_multi_series_iterator_dtor(void* iterator) {
  std::destroy_at(static_cast<entrypoint::series_data::MultiSeriesDecodeIterator*>(iterator));
}
