#include "samples_iterator.h"
#include "annotations.hpp"

#include "entrypoint/types/serialization.h"

/**
 * @brief Create a samples iterator for corresponding chunk_ref.
 *
 * @param args {
 *     iterator       uintptr // pointer to caller-owned iterator storage.
 *     serializedData uintptr // pointer to serialized data.
 *     chunk_ref uint32 // inner chunk id.
 * }
 *
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_serialization_serialized_data_samples_iterator_ctor(void* args) {
  struct Arguments {
    entrypoint::types::SamplesIterator* iterator;
    entrypoint::types::SerializedDataPtr serialized_data;
    uint32_t chunk_ref;
  };

  const auto in = static_cast<Arguments*>(args);
  std::visit([in](auto& serialized_data) { std::construct_at(in->iterator, serialized_data.samples_iterator(in->chunk_ref)); }, *in->serialized_data);
}

/**
 * @brief Advance samples iterator.
 *
 * @param args uintptr // pointer to samples iterator
 *
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_serialization_serialized_data_samples_iterator_next(void* args) {
  using Arguments = entrypoint::types::SamplesIterator;

  using series_data::decoder::DecodeIteratorSentinel;

  ++(*static_cast<Arguments*>(args));
}

/**
 * @brief Advance samples iterator until referenced sample is gte targetTimestamp.
 *
 * @param args {
 *     iterator uintptr // pointer to samples iterator
 *     targetTimestamp int64 // target timestamp
 * }
 *
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_serialization_serialized_data_samples_iterator_seek(void* args) {
  using series_data::decoder::DecodeIteratorSentinel;

  struct Arguments {
    entrypoint::types::SamplesIterator* iterator;
    int64_t target_timestamp;
  };

  const Arguments* in = static_cast<Arguments*>(args);
  in->iterator->seek_to(in->target_timestamp);
}

/**
 * @brief Reset a samples iterator for corresponding chunk_ref.
 *
 * @param args {
 *     iterator uintptr // pointer to caller-owned iterator storage
 *     serialized_data uintptr // pointer to serialized data
 *     chunk_ref uint32 // inner chunk id
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_serialization_serialized_data_samples_iterator_reset(void* args) {
  struct Arguments {
    entrypoint::types::SamplesIterator* iterator;
    entrypoint::types::SerializedDataPtr serialized_data;
    uint32_t chunk_ref;
  };

  const Arguments* in = static_cast<Arguments*>(args);
  std::visit([in](auto& serialized_data) { *in->iterator = serialized_data.samples_iterator(in->chunk_ref); }, *in->serialized_data);
}
