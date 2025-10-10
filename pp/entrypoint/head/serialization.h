#pragma once

#include "series_data/serialization/serialized_data.h"

namespace entrypoint::head {

class SerializedDataGo {
 public:
  explicit SerializedDataGo(const series_data::DataStorage& storage, const series_data::querier::QueriedChunkList& queried_chunks)
      : data_{series_data::serialization::DataSerializer{storage}.serialize(queried_chunks)} {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto get_buffer() const noexcept { return data_view_.get_buffer(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE auto get_chunks() const noexcept { return data_view_.get_chunks(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE uint32_t next() noexcept { return data_view_.next_series(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE auto iterator() const noexcept { return data_view_.create_current_series_iterator(); }

 private:
  series_data::serialization::SerializedData data_;
  series_data::serialization::SerializedDataView data_view_{data_};
};

using SerializedDataPtr = std::unique_ptr<SerializedDataGo>;
using SerializedDataIteratorPtr = std::unique_ptr<series_data::serialization::SerializedDataView::SeriesIterator>;

static_assert(sizeof(SerializedDataPtr) == sizeof(void*));
static_assert(sizeof(SerializedDataIteratorPtr) == sizeof(void*));

}  // namespace entrypoint::head