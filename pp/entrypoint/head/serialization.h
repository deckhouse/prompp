#pragma once

#include "primitives/go_slice.h"
#include "primitives/primitives.h"
#include "prometheus/query.h"
#include "series_data/decoder/decorator/downsampling_decode_iterator.h"
#include "series_data/serialization/serialized_data.h"

namespace entrypoint::series_data {

using DecodeIterator = ::series_data::decoder::decorator::DownsamplingDecodeIterator<::series_data::decoder::UniversalDecodeIterator>;
using SerializedDataIterator = ::series_data::serialization::SerializedDataView::SeriesIterator<DecodeIterator>;

class SerializedDataGo {
 public:
  explicit SerializedDataGo(const ::series_data::DataStorage& storage,
                            const ::series_data::querier::QueriedChunkList& queried_chunks,
                            PromPP::Prometheus::SelectHints&& select_hints,
                            PromPP::Primitives::Timestamp downsampling_ms)
      : data_{::series_data::serialization::DataSerializer{storage}.serialize(queried_chunks)},
        select_hints_(std::move(select_hints)),
        downsampling_ms_(downsampling_ms) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto get_buffer_view() const noexcept { return data_view_.get_buffer_view(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE auto get_chunks_view() const noexcept { return data_view_.get_chunks_view(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto next() noexcept { return data_view_.next_series(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE SerializedDataIterator iterator(uint32_t chunk_id) const noexcept {
    return data_view_.create_series_iterator<DecodeIterator>(chunk_id, DecodeIterator(downsampling_ms_));
  }

 private:
  ::series_data::serialization::SerializedData data_;
  ::series_data::serialization::SerializedDataView data_view_{data_};
  const PromPP::Prometheus::SelectHints select_hints_;
  PromPP::Primitives::Timestamp downsampling_ms_{};
};

using SerializedDataPtr = std::unique_ptr<SerializedDataGo>;

static_assert(sizeof(SerializedDataPtr) == sizeof(void*));

}  // namespace entrypoint::series_data