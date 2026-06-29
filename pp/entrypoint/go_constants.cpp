#include "go_constants.h"

#include "metrics/storage.h"
#include "prometheus/relabeler.h"
#include "series_data/serialization.h"
#include "wal/output_decoder.h"
#include "wal/segment_samples_storage.h"

namespace {

static_assert(sizeof(std::vector<char>) == Sizeof_StdVector);
static_assert(sizeof(BareBones::Vector<char>) == Sizeof_BareBonesVector);
static_assert(sizeof(roaring::Roaring) == Sizeof_RoaringBitset);

static_assert(sizeof(PromPP::Prometheus::Relabel::InnerSeries) == Sizeof_InnerSeries);

static_assert(sizeof(entrypoint::series_data::SamplesIterator) == Sizeof_SerializedDataSamplesIterator);
static_assert(sizeof(entrypoint::series_data::AggregationIterator) == Sizeof_SerializedDataAggregationIterator);

static_assert(sizeof(metrics::Storage::Iterator) == Sizeof_MetricsIterator);

static_assert(sizeof(PromPP::WAL::SegmentSamplesStorage) == Sizeof_SegmentSamplesStorage);
static_assert(sizeof(PromPP::WAL::ProtobufEncoder) == Sizeof_RemoteWriteMessageEncoder);
static_assert(sizeof(PromPP::WAL::SegmentSamplesStorageList::Iterator) == Sizeof_SegmentSamplesStorageListIterator);

}  // namespace