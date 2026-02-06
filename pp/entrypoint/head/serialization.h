#pragma once

#include "entrypoint/series_data/decode_iterator.h"
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
                            PromPP::Prometheus::SelectHints&& select_hints,
                            PromPP::Primitives::Timestamp downsampling_ms)
      : data_{::series_data::serialization::DataSerializer{storage}.serialize(queried_chunks)},
        select_hints_(std::move(select_hints)),
        downsampling_ms_(downsampling_ms) {}

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto get_buffer_view() const noexcept { return data_view_.get_buffer_view(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE auto get_chunks_view() const noexcept { return data_view_.get_chunks_view(); }

  [[nodiscard]] PROMPP_ALWAYS_INLINE auto next() noexcept { return data_view_.next_series(); }
  [[nodiscard]] PROMPP_ALWAYS_INLINE SerializedDataIterator iterator(uint32_t chunk_id) const noexcept {
    if (downsampling_ms_ != ::series_data::decoder::decorator::kNoDownsampling) [[unlikely]] {
      return data_view_.create_series_iterator<DecodeIterator>(chunk_id,
                                                               DecodeIterator(std::in_place_type<DecodeIterator::DownsamplingIterator>, downsampling_ms_));
    }

    if (select_hints_.func == "rate" || select_hints_.func == "increase") [[likely]] {
      return data_view_.create_series_iterator<DecodeIterator>(chunk_id,
                                                               DecodeIterator(std::in_place_type<DecodeIterator::RateIterator>, select_hints_.interval));
    }

    if (select_hints_.func == "irate" || select_hints_.func == "idelta") [[unlikely]] {
      return data_view_.create_series_iterator<DecodeIterator>(chunk_id,
                                                               DecodeIterator(std::in_place_type<DecodeIterator::IRateIterator>, select_hints_.interval));
    }

    if (select_hints_.func == "min_over_time") [[unlikely]] {
      return data_view_.create_series_iterator<DecodeIterator>(chunk_id,
                                                               DecodeIterator(std::in_place_type<DecodeIterator::MinOverTimeIterator>, select_hints_.interval));
    }

    if (select_hints_.func == "max_over_time") [[unlikely]] {
      return data_view_.create_series_iterator<DecodeIterator>(chunk_id,
                                                               DecodeIterator(std::in_place_type<DecodeIterator::MaxOverTimeIterator>, select_hints_.interval));
    }

    if (select_hints_.func == "last_over_time") [[unlikely]] {
      return data_view_.create_series_iterator<DecodeIterator>(
          chunk_id, DecodeIterator(std::in_place_type<DecodeIterator::LastOverTimeIterator>, select_hints_.interval));
    }

    if (select_hints_.func == "sum_over_time") [[unlikely]] {
      return data_view_.create_series_iterator<DecodeIterator>(chunk_id,
                                                               DecodeIterator(std::in_place_type<DecodeIterator::SumOverTimeIterator>, select_hints_.interval));
    }

    if (select_hints_.func == "changes") [[unlikely]] {
      return data_view_.create_series_iterator<DecodeIterator>(chunk_id,
                                                               DecodeIterator(std::in_place_type<DecodeIterator::ChangesIterator>, select_hints_.interval));
    }

    if (select_hints_.func == "delta") [[unlikely]] {
      return data_view_.create_series_iterator<DecodeIterator>(chunk_id,
                                                               DecodeIterator(std::in_place_type<DecodeIterator::DeltaIterator>, select_hints_.interval));
    }

    if (select_hints_.func == "resets") [[unlikely]] {
      return data_view_.create_series_iterator<DecodeIterator>(chunk_id,
                                                               DecodeIterator(std::in_place_type<DecodeIterator::ResetsIterator>, select_hints_.interval));
    }

    return data_view_.create_series_iterator<DecodeIterator>(chunk_id, DecodeIterator(std::in_place_type<DecodeIterator::UniversalDecodeIterator>));
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