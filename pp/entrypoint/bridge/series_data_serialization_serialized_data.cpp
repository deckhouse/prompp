#include "series_data_serialization_serialized_data.h"
#include "annotations.h"

#include "entrypoint/types/serialized_data.h"

/**
 * @brief Get next series_id in serialized data.
 *
 * @param args {
 *     serializedData uintptr // pointer to serialized data.
 * }
 *
 * @param res {
 *     series_id uint32 // series id (UINT32_MAX if no more series).
 *     chunk_ref uint32 // inner chunk id.
 * }
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_serialization_serialized_data_next(void* args, void* res) {
  struct Arguments {
    entrypoint::types::SerializedDataPtr serialized_data;
  };

  using Result = struct {
    uint32_t series_id;
    uint32_t chunk_ref;
  };
  const auto out = new (res) Result{};
  std::tie(out->series_id, out->chunk_ref) = static_cast<Arguments*>(args)->serialized_data->next();
}

/**
 * @brief Create a decode iterator for corresponding chunk_ref.
 *
 * @param args {
 *     serializedData uintptr // pointer to serialized data.
 *     chunk_ref uint32 // inner chunk id.
 * }
 *
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_serialization_serialized_data_iterator_ctor(void* args) {
  struct Arguments {
    entrypoint::types::SerializedDataIterator* iterator;
    entrypoint::types::SerializedDataPtr serialized_data;
    uint32_t chunk_ref;
  };

  const auto in = static_cast<Arguments*>(args);
  new (in->iterator) entrypoint::types::SerializedDataIterator(in->serialized_data->iterator(in->chunk_ref));
}

/**
 * @brief Advance decode iterator.
 *
 * @param iterator uintptr // pointer to decode iterator
 *
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_serialization_serialized_data_iterator_next(void* args) {
  using series_data::decoder::DecodeIteratorSentinel;
  using Arguments = entrypoint::types::SerializedDataIterator;

  ++(*static_cast<Arguments*>(args));
}

/**
 * @brief Advance decode iterator until referenced sample is gte targetTimestamp.
 *
 * @param args {
 *     iterator uintptr // pointer to decode iterator
 *     targetTimestamp int64 // target timestamp
 * }
 *
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_serialization_serialized_data_iterator_seek(void* args) {
  using series_data::decoder::DecodeIteratorSentinel;

  struct Arguments {
    entrypoint::types::SerializedDataIterator* iterator;
    int64_t target_timestamp;
  };

  const Arguments* in = static_cast<Arguments*>(args);
  in->iterator->seek_to(in->target_timestamp);
}

/**
 * @brief Reset a decode iterator for corresponding chunk_ref.
 *
 * @param args {
 *     serializedData uintptr // pointer to serialized data.
 *     iterator uintptr // pointer to decode iterator
 *     chunkRef uint32 // inner chunk id.
 * }
 *
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_serialization_serialized_data_iterator_reset(void* args) {
  struct Arguments {
    entrypoint::types::SerializedDataIterator* iterator;
    entrypoint::types::SerializedDataPtr serialized_data;
    uint32_t chunk_ref;
  };

  const Arguments* in = static_cast<Arguments*>(args);
  in->iterator->reset(in->serialized_data->get_buffer_view(), in->serialized_data->get_chunks_view(), in->chunk_ref);
}

/**
 * @brief Destroy serialized data object.
 *
 * @param args {
 *     serializedData uintptr // pointer to serialized data.
 * }
 *
 */
extern "C" PROMPP(entrypoint, fastcgo) void prompp_series_data_serialization_serialized_data_dtor(void* args) {
  struct Arguments {
    entrypoint::types::SerializedDataPtr serialized_data;
  };

  static_cast<Arguments*>(args)->~Arguments();
}
