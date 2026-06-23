package cppbridge

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// DataStorage is Go wrapper around series_data::Data_storage.
type DataStorage struct {
	dataStorage    uintptr
	timeInterval   atomic.Pointer[TimeInterval]
	lastUpdateTime atomic.Int64
	m              sync.Mutex
}

// NewDataStorage - constructor.
func NewDataStorage() *DataStorage {
	ds := &DataStorage{
		dataStorage:  seriesDataDataStorageCtor(),
		timeInterval: atomic.Pointer[TimeInterval]{},
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

// TimeIntervalWithValidateCache gets time interval from [DataStorage] with validate cache.
func (ds *DataStorage) TimeIntervalWithValidateCache(cacheCheckIntervalMs int64) TimeInterval {
	now := time.Now().UnixMilli()
	if now-ds.lastUpdateTime.Load() > cacheCheckIntervalMs {
		// slow path
		ds.m.Lock()
		if now-ds.lastUpdateTime.Load() > cacheCheckIntervalMs {
			timeInterval := seriesDataDataStorageTimeInterval(ds.dataStorage)
			ds.timeInterval.Store(&timeInterval)
			ds.lastUpdateTime.Store(now)
		}
		ds.m.Unlock()

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

func (ds *DataStorage) Query(query DataStorageQuery, downsamplingMs int64, selectHints unsafe.Pointer) DataStorageQueryResult {
	sd := NewDataStorageSerializedData(ds)
	querier, status := seriesDataDataStorageQueryV2(ds.dataStorage, query, sd, downsamplingMs, selectHints)
	runtime.KeepAlive(selectHints)
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

// QueryFirstTimestamps fills timestamps with the first sample timestamp (Prometheus ms) for each series in seriesIDs
func (ds *DataStorage) QueryFirstTimestamps(seriesIDs []uint32, timestamps []int64, notFoundTimestampValue int64) {
	seriesDataDataStorageQueryFirstTimestamps(ds.dataStorage, notFoundTimestampValue, seriesIDs, timestamps)
	runtime.KeepAlive(ds)
}

func (ds *DataStorage) QueryFinal(queriers []uintptr) {
	seriesDataDataStorageQueryFinal(queriers)
	runtime.KeepAlive(queriers)
}
