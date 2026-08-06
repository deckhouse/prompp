#pragma once

#include <cstdint>
#include <memory>
#include <variant>

#include "bare_bones/preprocess.h"
#include "entrypoint/types/data_storage.h"
#include "series_data/querier/querier.h"
#include "series_data/serialization/serialized_data.h"

namespace entrypoint::types {

template <class DataStorage>
class SerializedDataGo {
  using Reallocator = DataStorage::Reallocator;
  using SerializedData = series_data::serialization::SerializedData<Reallocator>;

 public:
  explicit SerializedDataGo(const DataStorage& storage, const series_data::querier::QueriedChunkList& queried_chunks)
      : data_{series_data::serialization::DataSerializer<DataStorage>{storage}.serialize(queried_chunks)} {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto get_buffer_view() const noexcept { return data_view_.get_buffer_view(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE auto get_chunks_view() const noexcept { return data_view_.get_chunks_view(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto next() noexcept { return data_view_.next_series(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE auto iterator(uint32_t chunk_id) const noexcept { return data_view_.create_series_iterator(chunk_id); }

 private:
  SerializedData data_;
  series_data::serialization::SerializedDataView data_view_{data_};
};

using SerializedDataVariant = std::variant<SerializedDataGo<DataStorageWithArenas>, SerializedDataGo<DataStorageWithoutArenas>>;
using SerializedDataPtr = std::unique_ptr<SerializedDataVariant>;
using SerializedDataIterator = series_data::serialization::SerializedDataView::SeriesIterator;

static_assert(sizeof(SerializedDataPtr) == sizeof(void*));

}  // namespace entrypoint::types
