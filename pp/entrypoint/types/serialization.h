#pragma once

#include <memory>
#include <variant>

#include "aggregation_iterator.h"
#include "multiseries_decode_iterator.h"
#include "primitives/primitives.h"
#include "prometheus/query.h"
#include "series_data/serialization/serialized_data.h"

namespace entrypoint::types {

using SamplesIterator = ::series_data::serialization::SerializedDataView::SeriesIterator;

template <class DataStorage = ::series_data::DataStorage<>>
class SerializedDataGo {
  using Reallocator = typename DataStorage::Reallocator;
  using SerializedData = ::series_data::serialization::BasicSerializedData<Reallocator>;

 public:
  explicit SerializedDataGo(const DataStorage& storage,
                            const ::series_data::querier::QueriedChunkList& queried_chunks,
                            SelectHints&& select_hints,
                            PromPP::Primitives::Timestamp downsampling_ms)
      : data_{::series_data::serialization::DataSerializer<DataStorage>{storage}.serialize(queried_chunks)},
        select_hints_(std::move(select_hints)),
        downsampling_ms_(downsampling_ms) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto get_buffer_view() const noexcept { return data_view_.get_buffer_view(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE auto get_chunks_view() const noexcept { return data_view_.get_chunks_view(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto next() noexcept { return data_view_.next_series(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE SamplesIterator samples_iterator(uint32_t chunk_id) const noexcept { return data_view_.create_series_iterator(chunk_id); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE AggregationIterator aggregation_iterator(uint32_t chunk_id) const noexcept {
    return create_aggregation_iterator(data_view_.create_series_iterator(chunk_id), select_hints_, downsampling_ms_);
  }
  PROMPP_ALWAYS_INLINE void construct_multi_series_iterator(MultiSeriesDecodeIterator* iterator, std::span<const uint32_t> series_ids) const noexcept {
    return construct_multi_series_decode_iterator(iterator, select_hints_, series_ids, data_view_);
  }

  PROMPP_ALWAYS_INLINE void reset_multi_series_iterator(MultiSeriesDecodeIterator& iterator, std::span<const uint32_t> series_ids) const noexcept {
    iterator.reset(select_hints_.function_parameters, [&](auto& iterators) PROMPP_LAMBDA_INLINE {
      MultiSeriesDecodeIterator::create_series_iterators(select_hints_, series_ids, data_view_, iterators);
    });
  }

 private:
  SerializedData data_;
  ::series_data::serialization::SerializedDataView data_view_{data_};
  const SelectHints select_hints_;
  PromPP::Primitives::Timestamp downsampling_ms_{};
};

using SerializedDataVariant = std::variant<SerializedDataGo<::series_data::DataStorage<true>>, SerializedDataGo<::series_data::DataStorage<false>>>;
using SerializedDataPtr = std::unique_ptr<SerializedDataVariant>;

static_assert(sizeof(SerializedDataPtr) == sizeof(void*));

}  // namespace entrypoint::types