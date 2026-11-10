package cppbridge

import (
	"runtime"
	"sync/atomic"
	"unsafe"
)

// DataStorage is Go wrapper around series_data::Data_storage.
type DataStorage struct {
	dataStorage  uintptr
	timeInterval atomic.Pointer[TimeInterval]
}

// NewDataStorage - constructor.
func NewDataStorage(collectMetrics, useArenas bool) *DataStorage {
	ds := &DataStorage{
		dataStorage:  seriesDataDataStorageCtor(collectMetrics, useArenas),
		timeInterval: atomic.Pointer[TimeInterval]{},
	}
	ds.timeInterval.Store(newInvalidTimeIntervalPtr())

	runtime.SetFinalizer(ds, func(ds *DataStorage) {
		seriesDataDataStorageDtor(ds.dataStorage)
	})

	return ds
}

// TimeInterval returns the timestamp bounds of the stored data, [NewInvalidTimeInterval] if there is none.
//
// The bounds are cached: computing them from the storage walks every series and decodes the first
// timestamp of each. [DataStorage.EncodeInnerSeriesSlice] keeps the cache up to date, so a storage that is
// only appended to never needs a rescan.
//
// The cache of a storage filled through [HeadWalDecoder.DecodeToDataStorage], which does not maintain it,
// must be primed with invalidateCache before the first append: an invalid cache is indistinguishable from
// the cache of an empty storage, and extending it with the bounds of one appended batch would report the
// restored data as absent.
func (ds *DataStorage) TimeInterval(invalidateCache bool) TimeInterval {
	if invalidateCache || ds.timeInterval.Load().IsInvalid() {
		timeInterval := seriesDataDataStorageTimeInterval(ds.dataStorage)
		ds.timeInterval.Store(&timeInterval)
		runtime.KeepAlive(ds)
	}

	return *ds.timeInterval.Load()
}

func (ds *DataStorage) GetQueriedSeriesBitset() []byte {
	size := seriesDataDataStorageQueriedSeriesBitsetSize(ds.dataStorage)
	bitset := seriesDataDataStorageQueriedSeriesBitset(ds.dataStorage, make([]byte, 0, size))
	runtime.KeepAlive(ds)
	return bitset
}

func (ds *DataStorage) SetQueriedSeriesBitset(bitset []byte) bool {
	result := seriesDataDataStorageQueriedSeriesSetBitset(ds.dataStorage, bitset)
	runtime.KeepAlive(ds)
	return result
}

func (ds *DataStorage) Pointer() uintptr {
	return ds.dataStorage
}

func (ds *DataStorage) AllocatedMemory() uint64 {
	res := seriesDataDataStorageAllocatedMemory(ds.dataStorage)
	runtime.KeepAlive(ds)
	return res
}

// Encode encodes single triplet into the data storage.
func (ds *DataStorage) Encode(seriesID uint32, timestamp int64, value float64) {
	seriesDataEncoderEncode(ds.dataStorage, seriesID, timestamp, value)
	runtime.KeepAlive(ds)
}

// EncodeInnerSeriesSlice encodes InnerSeries slice produced by relabeler into the data storage
// and keeps the cached time interval in sync with the appended samples.
func (ds *DataStorage) EncodeInnerSeriesSlice(innerSeriesSlice []InnerSeries) {
	seriesDataEncoderEncodeInnerSeriesSlice(ds.dataStorage, innerSeriesSlice)
	runtime.KeepAlive(ds)

	appended := NewInvalidTimeInterval()
	for i := range innerSeriesSlice {
		interval := innerSeriesSlice[i].TimeInterval()
		appended.MinT = min(appended.MinT, interval.MinT)
		appended.MaxT = max(appended.MaxT, interval.MaxT)
	}

	ds.extendTimeInterval(appended)
}

// MergeOutOfOrderChunks merges out of order chunks in the data storage.
func (ds *DataStorage) MergeOutOfOrderChunks() {
	seriesDataEncoderMergeOutOfOrderChunks(ds.dataStorage)
	runtime.KeepAlive(ds)
}

type UnusedSeriesDataUnloader struct {
	unloader uintptr
	ds       *DataStorage
}

func (u *UnusedSeriesDataUnloader) CreateSnapshot() []byte {
	snapshot := seriesDataUnusedSeriesDataUnloaderCreateSnapshot(u.unloader)
	runtime.KeepAlive(u)
	return snapshot
}

func (u *UnusedSeriesDataUnloader) Unload() {
	seriesDataUnusedSeriesDataUnloaderUnload(u.unloader)
	runtime.KeepAlive(u)
}

func (ds *DataStorage) CreateUnusedSeriesDataUnloader() *UnusedSeriesDataUnloader {
	unloader := &UnusedSeriesDataUnloader{
		unloader: seriesDataUnusedSeriesDataUnloaderCtor(ds.dataStorage),
		ds:       ds,
	}

	runtime.SetFinalizer(unloader, func(u *UnusedSeriesDataUnloader) {
		seriesDataUnusedSeriesDataUnloaderDtor(u.unloader)
	})

	return unloader
}

// DataStorageQuery is the query as callers state it. LabelSetIDs carries the owner of
// the ids, so a C-allocated buffer cannot be freed while C++ reads it.
type DataStorageQuery struct {
	StartTimestampMs int64
	EndTimestampMs   int64
	LabelSetIDs      LabelSetIDs
}

// dataStorageQueryABI is the ABI counterpart: it is embedded verbatim into the bridge
// args struct, so its layout must stay exactly what the C++ side reads — a 16-byte
// TimeInterval followed by a 24-byte Go slice header. The asserts below fail to compile
// if that drifts, which neither the compiler nor go vet would otherwise notice.
type dataStorageQueryABI struct {
	StartTimestampMs int64
	EndTimestampMs   int64
	LabelSetIDs      []uint32
}

const (
	_ = uint(unsafe.Sizeof(dataStorageQueryABI{}) - 40)
	_ = uint(40 - unsafe.Sizeof(dataStorageQueryABI{}))
)

func (ds *DataStorage) Query(query DataStorageQuery, downsamplingMs int64, selectHints unsafe.Pointer) DataStorageQueryResult {
	defer query.LabelSetIDs.keepAlive()

	abi := dataStorageQueryABI{
		StartTimestampMs: query.StartTimestampMs,
		EndTimestampMs:   query.EndTimestampMs,
		LabelSetIDs:      query.LabelSetIDs.slice(),
	}

	sd := NewDataStorageSerializedData(ds)
	querier, status := seriesDataDataStorageQueryV2(ds.dataStorage, abi, sd, downsamplingMs, selectHints)
	runtime.KeepAlive(selectHints)
	return DataStorageQueryResult{
		Querier:        querier,
		Status:         status,
		SerializedData: sd,
	}
}

// InstantQuery finds, for each series in labelSetIDs, the sample closest to (at or before) targetTimestamp
// and writes it into the C-shared samples slice (pointed to by samples), in the same order as labelSetIDs.
func (ds *DataStorage) InstantQuery(targetTimestamp int64, labelSetIDs LabelSetIDs, samples uintptr) DataStorageQueryResult {
	defer labelSetIDs.keepAlive()

	r := seriesDataDataStorageInstantQuery(ds.dataStorage, labelSetIDs.slice(), targetTimestamp, samples)
	runtime.KeepAlive(ds)
	return r
}

// QueryStaleNaNSeries fills the first sample timestamp (Prometheus ms) and the series id for each
// series in seriesIDs directly into the C-shared series slice (pointed to by series).
func (ds *DataStorage) QueryStaleNaNSeries(seriesIDs LabelSetIDs, series uintptr) {
	defer seriesIDs.keepAlive()

	seriesDataDataStorageQueryStaleNaNSeries(ds.dataStorage, seriesIDs.slice(), series)
	runtime.KeepAlive(ds)
}

func (ds *DataStorage) QueryFinal(queriers []uintptr) {
	seriesDataDataStorageQueryFinal(queriers)
	runtime.KeepAlive(queriers)
}

// extendTimeInterval widens the cached time interval to cover interval. The bounds of the data
// only ever grow, so the cache is never narrowed.
func (ds *DataStorage) extendTimeInterval(interval TimeInterval) {
	if interval.IsInvalid() {
		return
	}

	for {
		cached := ds.timeInterval.Load()
		minT := min(cached.MinT, interval.MinT)
		maxT := max(cached.MaxT, interval.MaxT)

		// Return before taking the address of an interval, so the common case of an append that widens
		// nothing does not move one to the heap.
		if minT == cached.MinT && maxT == cached.MaxT {
			return
		}

		if ds.timeInterval.CompareAndSwap(cached, &TimeInterval{MinT: minT, MaxT: maxT}) {
			return
		}
	}
}
