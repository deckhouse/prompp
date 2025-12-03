package cppbridge

import (
	"runtime"
	"sync/atomic"
)

// DataStorage is Go wrapper around series_data::Data_storage.
type DataStorage struct {
	dataStorage       uintptr
	gcDestroyDetector *uint64
	timeInterval      atomic.Pointer[TimeInterval]
}

// NewDataStorage - constructor.
func NewDataStorage() *DataStorage {
	ds := &DataStorage{
		dataStorage:       seriesDataDataStorageCtor(),
		gcDestroyDetector: &gcDestroyDetector,
		timeInterval:      atomic.Pointer[TimeInterval]{},
	}
	ds.timeInterval.Store(newInvalidTimeIntervalPtr())

	runtime.SetFinalizer(ds, func(ds *DataStorage) {
		seriesDataDataStorageDtor(ds.dataStorage)
	})

	return ds
}

// Reset - resets data storage.
func (ds *DataStorage) Reset() {
	seriesDataDataStorageReset(ds.dataStorage)
	ds.timeInterval.Store(newInvalidTimeIntervalPtr())
}

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

type DataStorageQuery struct {
	StartTimestampMs int64
	EndTimestampMs   int64
	LabelSetIDs      []uint32
}

func (ds *DataStorage) Query(query DataStorageQuery, downsamplingMs int64) DataStorageQueryResult {
	sd := NewDataStorageSerializedData(ds)
	querier, status := seriesDataDataStorageQueryV2(ds.dataStorage, query, sd, downsamplingMs)
	return DataStorageQueryResult{
		Querier:        querier,
		Status:         status,
		SerializedData: sd,
	}
}

// InstantQuery .
// Deprecated: InstantQuery .
func (ds *DataStorage) InstantQuery(targetTimestamp int64, labelSetIDs []uint32, samples uintptr) DataStorageQueryResult {
	return seriesDataDataStorageInstantQuery(ds.dataStorage, labelSetIDs, targetTimestamp, samples)
}

func (ds *DataStorage) QueryFinal(queriers []uintptr) {
	seriesDataDataStorageQueryFinal(queriers)
	runtime.KeepAlive(queriers)
}
