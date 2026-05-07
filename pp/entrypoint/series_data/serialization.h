#pragma once

#include "decode_iterator.h"
#include "entrypoint/series_data/multiseries_decode_iterator.h"
#include "primitives/go_slice.h"
#include "primitives/primitives.h"
#include "prometheus/query.h"
#include "series_data/serialization/serialized_data.h"

namespace entrypoint::series_data {

using SerializedDataIterator = ::series_data::serialization::SerializedDataView::SeriesIterator<DecodeIterator>;

class SerializedDataGo {
 public:
  explicit SerializedDataGo(const ::series_data::DataStorage& storage,
                            const ::series_data::querier::QueriedChunkList& queried_chunks,
                            SelectHints&& select_hints,
                            PromPP::Primitives::Timestamp downsampling_ms)
      : data_{::series_data::serialization::DataSerializer{storage}.serialize(queried_chunks)},
        select_hints_(std::move(select_hints)),
        downsampling_ms_(downsampling_ms) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto get_buffer_view() const noexcept { return data_view_.get_buffer_view(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE auto get_chunks_view() const noexcept { return data_view_.get_chunks_view(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto next() noexcept { return data_view_.next_series(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE SerializedDataIterator iterator(uint32_t chunk_id) const noexcept {
    return data_view_.create_series_iterator<DecodeIterator>(chunk_id, create_decode_iterator(select_hints_, downsampling_ms_));
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE MultiSeriesDecodeIterator multi_series_iterator(std::span<const uint32_t> series_ids) const noexcept {
    return create_multi_series_decode_iterator(select_hints_, series_ids, data_view_);
  }

 private:
  ::series_data::serialization::SerializedData data_;
  ::series_data::serialization::SerializedDataView data_view_{data_};
  const SelectHints select_hints_;
  PromPP::Primitives::Timestamp downsampling_ms_{};
};

using SerializedDataPtr = std::unique_ptr<SerializedDataGo>;

static_assert(sizeof(SerializedDataPtr) == sizeof(void*));

}  // namespace entrypoint::series_data