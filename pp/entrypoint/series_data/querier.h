#pragma once

#include "bare_bones/bitset.h"
#include "primitives/go_slice.h"
#include "primitives/primitives.h"
#include "primitives/sample.h"
#include "series_data/querier/instant_querier.h"
#include "series_data/querier/querier.h"
#include "series_data/serialization/serializer.h"

namespace entrypoint::series_data {
template <class Querier>
concept QuerierInterface = requires(Querier querier) {
  { querier.query() };
  { querier.query_finalize() };
  { querier.series_to_load() } -> std::same_as<const BareBones::Bitset&>;
  { querier.need_loading() } -> std::same_as<bool>;
};

template <typename LsIDStorage, typename SampleStorage>
class InstantQuerierWithArgumentsWrapper {
  using Timestamp = PromPP::Primitives::Timestamp;
  using DataStorage = ::series_data::DataStorage;

 public:
  InstantQuerierWithArgumentsWrapper(DataStorage& storage, SampleStorage& samples, const LsIDStorage& label_set_ids, const Timestamp& timestamp)
      : samples_(samples), label_set_ids_(label_set_ids), timestamp_(timestamp), instant_querier_(storage) {}

  void query() noexcept { instant_querier_.query(samples_, label_set_ids_, timestamp_); }

  void query_finalize() noexcept { instant_querier_.query_reload(samples_, label_set_ids_, timestamp_); }

  [[nodiscard]] const BareBones::Bitset& series_to_load() const noexcept { return instant_querier_.get_series_to_load(); }

  [[nodiscard]] bool need_loading() const noexcept { return instant_querier_.need_loading(); }

 private:
  SampleStorage& samples_;
  const LsIDStorage& label_set_ids_;
  const Timestamp timestamp_;

  ::series_data::InstantQuerier instant_querier_;
};

class RangeQuerierWithArgumentsWrapper {
  using DataStorage = ::series_data::DataStorage;
  using LabelSetID = PromPP::Primitives::LabelSetID;
  template <class T>
  using Slice = PromPP::Primitives::Go::Slice<T>;
  using Query = ::series_data::querier::Query<Slice<LabelSetID>>;
  using Serializer = ::series_data::serialization::Serializer;
  using BytesStream = PromPP::Primitives::Go::BytesStream;

 public:
  RangeQuerierWithArgumentsWrapper(Slice<char>* serialized_chunks, DataStorage& storage, const Query& query)
      : serialized_chunks_(serialized_chunks), query_(query), querier_(storage) {}

  void query() noexcept { queried_chunk_list_ = querier_.query(query_); }

  void query_finalize() const noexcept {
    Serializer serializer{querier_.get_storage()};
    PromPP::Primitives::Go::BytesStream bytes_stream{serialized_chunks_};
    serializer.serialize(queried_chunk_list_, bytes_stream);
  }

  [[nodiscard]] const BareBones::Bitset& series_to_load() const noexcept { return querier_.get_series_to_load(); }

  [[nodiscard]] bool need_loading() const noexcept { return querier_.need_loading(); }

 private:
  Slice<char>* serialized_chunks_;
  const Query& query_;
  ::series_data::querier::QueriedChunkList queried_chunk_list_{};
  ::series_data::querier::Querier querier_;
};

}  // namespace entrypoint::series_data

static_assert(entrypoint::series_data::QuerierInterface<
              entrypoint::series_data::InstantQuerierWithArgumentsWrapper<PromPP::Primitives::Go::SliceView<PromPP::Primitives::LabelSetID>,
                                                                          PromPP::Primitives::Go::SliceView<PromPP::Primitives::Sample>>>);

static_assert(entrypoint::series_data::QuerierInterface<entrypoint::series_data::RangeQuerierWithArgumentsWrapper>);
