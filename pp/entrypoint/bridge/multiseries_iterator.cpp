#include "multiseries_iterator.h"
#include "annotations.hpp"

#include "entrypoint/types/serialization.h"

/**
 * @brief Construct a multi-series decode iterator over the given series ids.
 *
 * @param args {
 *     iterator uintptr // pointer to storage of size Sizeof_MultiSeriesDecodeIterator (placement new).
 *     serializedData uintptr // pointer to serialized data.
 *     seriesIDs []uint32 // slice view of series ids to use in iterator.
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_serialization_serialized_data_multi_series_iterator_ctor(void* args) {
  struct Arguments {
    entrypoint::types::MultiSeriesDecodeIterator* iterator;
    entrypoint::types::SerializedDataPtr serialized_data;
    PromPP::Primitives::Go::SliceView<uint32_t> series_ids;
  };

  const auto in = static_cast<Arguments*>(args);
  std::visit([in](auto& serialized_data) { serialized_data.construct_multi_series_iterator(in->iterator, in->series_ids.span()); }, *in->serialized_data);
}

/**
 * @brief Reset a multi-series decode iterator into the given series ids.
 *
 * @param args {
 *     iterator uintptr // pointer to a constructed MultiSeriesDecodeIterator.
 *     serializedData uintptr // pointer to serialized data.
 *     seriesIDs []uint32 // slice view of series ids to use in iterator.
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_serialization_serialized_data_multi_series_iterator_reset(void* args) {
  struct Arguments {
    entrypoint::types::MultiSeriesDecodeIterator* iterator;
    entrypoint::types::SerializedDataPtr serialized_data;
    PromPP::Primitives::Go::SliceView<uint32_t> series_ids;
  };

  const auto in = static_cast<Arguments*>(args);
  std::visit([in](auto& serialized_data) { serialized_data.reset_multi_series_iterator(*in->iterator, in->series_ids.span()); }, *in->serialized_data);
}

/**
 * @brief Advance multi-series decode iterator.
 *
 * @param args uintptr // pointer to multi-series decode iterator
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_serialization_serialized_data_multi_series_iterator_next(void* args) {
  using Arguments = entrypoint::types::MultiSeriesDecodeIterator;

  ++(*static_cast<Arguments*>(args));
}

/**
 * @brief Destroy multi-series decode iterator (call before reusing).
 *
 * @param args uintptr // pointer to multi-series decode iterator
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_serialization_serialized_data_multi_series_iterator_dtor(void* args) {
  using Arguments = entrypoint::types::MultiSeriesDecodeIterator;

  std::destroy_at(static_cast<Arguments*>(args));
}
