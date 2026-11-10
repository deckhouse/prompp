#include "aggregation_iterator.h"
#include "annotations.hpp"

#include "entrypoint/types/serialization.h"

/**
 * @brief Create a aggregation iterator for corresponding chunk_ref.
 *
 * @param args {
 *     iterator       uintptr // pointer to caller-owned iterator storage.
 *     serializedData uintptr // pointer to serialized data.
 *     chunk_ref uint32 // inner chunk id.
 * }
 *
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_serialization_serialized_data_aggregation_iterator_ctor(void* args) {
  struct Arguments {
    entrypoint::types::AggregationIterator* iterator;
    entrypoint::types::SerializedDataPtr serialized_data;
    uint32_t chunk_ref;
  };

  const auto in = static_cast<Arguments*>(args);
  std::visit([in](auto& serialized_data) { std::construct_at(in->iterator, serialized_data.aggregation_iterator(in->chunk_ref)); }, *in->serialized_data);
}

/**
 * @brief Advance aggregation iterator.
 *
 * @param args uintptr // pointer to aggregation iterator
 *
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_serialization_serialized_data_aggregation_iterator_next(void* args) {
  using Arguments = entrypoint::types::AggregationIterator;

  using series_data::decoder::DecodeIteratorSentinel;

  ++(*static_cast<Arguments*>(args));
}

/**
 * @brief Reset a aggregation iterator for corresponding chunk_ref.
 *
 * @param args {
 *     iterator uintptr // pointer to caller-owned iterator storage
 *     serialized_data uintptr // pointer to serialized data
 *     chunk_ref uint32 // inner chunk id
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_serialization_serialized_data_aggregation_iterator_reset(void* args) {
  struct Arguments {
    entrypoint::types::AggregationIterator* iterator;
    entrypoint::types::SerializedDataPtr serialized_data;
    uint32_t chunk_ref;
  };

  const Arguments* in = static_cast<Arguments*>(args);
  std::visit([in](auto& serialized_data) { *in->iterator = serialized_data.aggregation_iterator(in->chunk_ref); }, *in->serialized_data);
}
