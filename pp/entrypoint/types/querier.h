#pragma once

#include <concepts>
#include <cstdint>
#include <memory>
#include <span>
#include <variant>

#include "bare_bones/bitset.h"
#include "bare_bones/preprocess.h"
#include "entrypoint/types/go_constants.h"
#include "entrypoint/types/serialized_data.h"
#include "primitives/go_slice.h"
#include "primitives/primitives.h"
#include "series_data/data_storage.h"
#include "series_data/encoder/sample.h"
#include "series_data/querier/instant_querier.h"
#include "series_data/querier/querier.h"
#include "series_data/querier/query.h"

namespace entrypoint::types {

template <class Querier>
concept QuerierInterface = requires(Querier querier) {
  typename Querier::Storage;
  { querier.query() };
  { querier.query_finalize() };
  { querier.series_to_load() } -> std::same_as<const BareBones::Bitset&>;
  { querier.need_loading() } -> std::same_as<bool>;
  { querier.storage() } -> std::same_as<typename Querier::Storage&>;
};

enum class QueryStatus : uint8_t {
  kSuccess = 0,
  kNeedDataLoad,
};

template <typename LsIDStorage, typename SampleStorage, class DataStorage>
class InstantQuerierWithArgumentsWrapper {
  using Timestamp = PromPP::Primitives::Timestamp;

 public:
  using Storage = DataStorage;

  InstantQuerierWithArgumentsWrapper(DataStorage& storage, const LsIDStorage& label_set_ids, const Timestamp& timestamp, SampleStorage& samples)
      : instant_querier_(storage), samples_(samples), label_set_ids_(label_set_ids), timestamp_(timestamp) {}

  void query() noexcept { instant_querier_.query(samples_, label_set_ids_, timestamp_); }
  void query_finalize() noexcept { instant_querier_.query_reload(samples_, label_set_ids_, timestamp_); }

  [[nodiscard]] const BareBones::Bitset& series_to_load() const noexcept { return instant_querier_.get_series_to_load(); }
  [[nodiscard]] bool need_loading() const noexcept { return instant_querier_.need_loading(); }
  [[nodiscard]] DataStorage& storage() noexcept { return instant_querier_.get_storage(); }

 private:
  ::series_data::InstantQuerier<DataStorage> instant_querier_;
  SampleStorage samples_;
  const LsIDStorage label_set_ids_;
  const Timestamp timestamp_;
};

struct SampleWithGoLabels : public ::series_data::encoder::Sample {
  // series_id is filled by the instant querier alongside the sample; go_labels_ is written
  // from the Go side (querier.InstantSeries.LabelSet). This layout mirrors the Go struct.
  uint32_t series_id;

 private:
  char go_labels_[Sizeof_GoLabels];
};

struct StaleNaNSeriesWithGoLabels {
  // timestamp and series_id are filled by the data storage stalenan query; go_labels_ is
  // written from the Go side (querier.StaleNaNSeries.labelSet). This layout mirrors the Go struct.
  PromPP::Primitives::Timestamp timestamp;
  uint32_t series_id;

 private:
  char go_labels_[Sizeof_GoLabels];
};

using InstantQuerierWithArgumentsWrapperEntrypoint =
    InstantQuerierWithArgumentsWrapper<PromPP::Primitives::Go::SliceView<PromPP::Primitives::LabelSetID>, std::span<SampleWithGoLabels>, DataStorageWithArenas>;
using InstantQuerierWithArgumentsWrapperWithoutArenasEntrypoint =
    InstantQuerierWithArgumentsWrapper<PromPP::Primitives::Go::SliceView<PromPP::Primitives::LabelSetID>,
                                       std::span<SampleWithGoLabels>,
                                       DataStorageWithoutArenas>;

template <class DataStorage>
class BasicRangeQuerierWithArgumentsWrapperV2 {
  using LabelSetID = PromPP::Primitives::LabelSetID;
  template <class T>
  using Slice = PromPP::Primitives::Go::Slice<T>;
  using Query = ::series_data::querier::Query<Slice<LabelSetID>>;
  using BytesStream = PromPP::Primitives::Go::BytesStream;

 public:
  using Storage = DataStorage;

  BasicRangeQuerierWithArgumentsWrapperV2(DataStorage& storage, const Query& query, SerializedDataPtr* serialized_data)
      : querier_(storage), query_(&query), serialized_data_(serialized_data) {}

  void query() noexcept {
    querier_.query(*query_);
    if (!querier_.need_loading()) {
      serialize_chunks();
    }
  }

  PROMPP_ALWAYS_INLINE void query_finalize() const noexcept { serialize_chunks(); }

  [[nodiscard]] const BareBones::Bitset& series_to_load() const noexcept { return querier_.get_series_to_load(); }
  [[nodiscard]] bool need_loading() const noexcept { return querier_.need_loading(); }
  [[nodiscard]] DataStorage& storage() noexcept { return querier_.get_storage(); }

 private:
  ::series_data::querier::Querier<DataStorage> querier_;
  const Query* query_;
  SerializedDataPtr* serialized_data_;

  PROMPP_ALWAYS_INLINE void serialize_chunks() const noexcept {
    std::construct_at(serialized_data_,
                      std::make_unique<SerializedDataVariant>(std::in_place_type<SerializedDataGo<DataStorage>>, querier_.get_storage(), querier_.chunks()));
  }
};

enum class QuerierType : uint8_t { kInstantQuerier = 0, kRangeQuerier, kRangeQuerierV2 };

using RangeQuerierWithArgumentsWrapperV2 = BasicRangeQuerierWithArgumentsWrapperV2<DataStorageWithArenas>;
using RangeQuerierWithArgumentsWrapperWithoutArenasV2 = BasicRangeQuerierWithArgumentsWrapperV2<DataStorageWithoutArenas>;

using QuerierVariant = std::variant<InstantQuerierWithArgumentsWrapperEntrypoint,
                                    RangeQuerierWithArgumentsWrapperV2,
                                    InstantQuerierWithArgumentsWrapperWithoutArenasEntrypoint,
                                    RangeQuerierWithArgumentsWrapperWithoutArenasV2>;
using QuerierVariantPtr = std::unique_ptr<QuerierVariant>;

}  // namespace entrypoint::types

static_assert(entrypoint::types::QuerierInterface<entrypoint::types::InstantQuerierWithArgumentsWrapperEntrypoint>);
static_assert(entrypoint::types::QuerierInterface<entrypoint::types::RangeQuerierWithArgumentsWrapperV2>);
static_assert(entrypoint::types::QuerierInterface<entrypoint::types::InstantQuerierWithArgumentsWrapperWithoutArenasEntrypoint>);
static_assert(entrypoint::types::QuerierInterface<entrypoint::types::RangeQuerierWithArgumentsWrapperWithoutArenasV2>);
