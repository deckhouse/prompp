#pragma once

#include "bare_bones/bitset.h"
#include "entrypoint/go_constants.h"
#include "primitives/go_slice.h"
#include "primitives/primitives.h"
#include "series_data/querier/instant_querier.h"
#include "series_data/querier/querier.h"
#include "series_data/serialization/serialized_data.h"

namespace entrypoint::series_data {

template <class Querier>
concept QuerierInterface = requires(Querier querier) {
  { querier.query() };
  { querier.query_finalize() };
  { querier.series_to_load() } -> std::same_as<const BareBones::Bitset&>;
  { querier.need_loading() } -> std::same_as<bool>;
  { querier.storage() } -> std::same_as<head::DataStorage&>;
};

enum class QueryStatus : uint8_t {
  kSuccess = 0,
  kNeedDataLoad,
};

template <typename LsIDStorage, typename SampleStorage>
class InstantQuerierWithArgumentsWrapper {
  using Timestamp = PromPP::Primitives::Timestamp;
  using DataStorage = head::DataStorage;

 public:
  InstantQuerierWithArgumentsWrapper(DataStorage& storage, const LsIDStorage& label_set_ids, const Timestamp& timestamp, SampleStorage& samples)
      : instant_querier_(storage), samples_(samples), label_set_ids_(label_set_ids), timestamp_(timestamp) {}

  void query() noexcept { instant_querier_.query(samples_, label_set_ids_, timestamp_); }
  void query_finalize() noexcept { instant_querier_.query_reload(samples_, label_set_ids_, timestamp_); }

  [[nodiscard]] const BareBones::Bitset& series_to_load() const noexcept { return instant_querier_.get_series_to_load(); }
  [[nodiscard]] bool need_loading() const noexcept { return instant_querier_.need_loading(); }
  [[nodiscard]] DataStorage& storage() noexcept { return instant_querier_.get_storage(); }

 private:
  ::series_data::InstantQuerier<head::DataStorage> instant_querier_;
  SampleStorage samples_;
  const LsIDStorage label_set_ids_;
  const Timestamp timestamp_;
};

struct SampleWithGoLabels : public ::series_data::encoder::Sample {
 private:
  char go_labels_[Sizeof_GoLabels];
};

using InstantQuerierWithArgumentsWrapperEntrypoint = InstantQuerierWithArgumentsWrapper<PromPP::Primitives::Go::SliceView<PromPP::Primitives::LabelSetID>,
                                                                                        std::span<entrypoint::series_data::SampleWithGoLabels>>;

class RangeQuerierWithArgumentsWrapperV2 {
  using DataStorage = head::DataStorage;
  using LabelSetID = PromPP::Primitives::LabelSetID;
  template <class T>
  using Slice = PromPP::Primitives::Go::Slice<T>;
  using Query = ::series_data::querier::Query<Slice<LabelSetID>>;
  using BytesStream = PromPP::Primitives::Go::BytesStream;

 public:
  RangeQuerierWithArgumentsWrapperV2(DataStorage& storage, const Query& query, head::SerializedDataPtr* serialized_data)
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
  ::series_data::querier::Querier<head::DataStorage> querier_;
  const Query* query_;
  head::SerializedDataPtr* serialized_data_;

  PROMPP_ALWAYS_INLINE void serialize_chunks() const noexcept {
    std::construct_at(serialized_data_, std::make_unique<head::SerializedDataGo>(querier_.get_storage(), querier_.chunks()));
  }
};

enum class QuerierType : uint8_t { kInstantQuerier = 0, kRangeQuerier, kRangeQuerierV2 };

using QuerierVariant = std::variant<InstantQuerierWithArgumentsWrapperEntrypoint, RangeQuerierWithArgumentsWrapperV2>;
using QuerierVariantPtr = std::unique_ptr<QuerierVariant>;

}  // namespace entrypoint::series_data

static_assert(entrypoint::series_data::QuerierInterface<entrypoint::series_data::InstantQuerierWithArgumentsWrapperEntrypoint>);
static_assert(entrypoint::series_data::QuerierInterface<entrypoint::series_data::RangeQuerierWithArgumentsWrapperV2>);