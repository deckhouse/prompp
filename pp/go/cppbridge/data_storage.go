package cppbridge

import (
	"runtime"
	"sync/atomic"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/util"
)

var (
	dsCreate = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_cpp_objects_create_count",
			Help:        "Current number of created C++ objects.",
			ConstLabels: prometheus.Labels{"object": "data_storage"},
		},
	)

	dsFinalize = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_cpp_objects_finalize_count",
			Help:        "Current number of finalized C++ objects.",
			ConstLabels: prometheus.Labels{"object": "data_storage"},
		},
	)
)

// DataStorage is Go wrapper around series_data::Data_storage.
type DataStorage struct {
	dataStorage  uintptr
	timeInterval atomic.Pointer[TimeInterval]
}

// NewDataStorage - constructor.
func NewDataStorage(collectMetrics bool) *DataStorage {
	ds := &DataStorage{
		dataStorage:  seriesDataDataStorageCtor(collectMetrics),
		timeInterval: atomic.Pointer[TimeInterval]{},
	}
	ds.timeInterval.Store(newInvalidTimeIntervalPtr())

	runtime.AddCleanup(ds, func(pointer uintptr) {
		seriesDataDataStorageDtor(pointer)
		dsFinalize.Inc()
	}, ds.dataStorage)

	dsCreate.Inc()

	return ds
}

// Reset - resets data storage.
func (ds *DataStorage) Reset() {
	seriesDataDataStorageReset(ds.dataStorage)
	ds.timeInterval.Store(newInvalidTimeIntervalPtr())
	runtime.KeepAlive(ds)
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
	runtime.KeepAlive(ds)
	runtime.AddCleanup(unloader, seriesDataUnusedSeriesDataUnloaderDtor, unloader.unloader)

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
	runtime.KeepAlive(ds)
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
	result := seriesDataDataStorageInstantQuery(ds.dataStorage, labelSetIDs, targetTimestamp, samples)
	runtime.KeepAlive(ds)
	return result
}

// QueryFirstTimestamps fills timestamps with the first sample timestamp (Prometheus ms) for each series in seriesIDs.
func (ds *DataStorage) QueryFirstTimestamps(seriesIDs []uint32, timestamps []int64) {
	seriesDataDataStorageQueryFirstTimestamps(ds.dataStorage, seriesIDs, timestamps)
	runtime.KeepAlive(ds)
}

// QueryStaleNaNSeries fills the first sample timestamp (Prometheus ms) and the series id for each
// series in seriesIDs directly into the C-shared series slice (pointed to by series).
func (ds *DataStorage) QueryStaleNaNSeries(seriesIDs []uint32, series uintptr) {
	seriesDataDataStorageQueryStaleNaNSeries(ds.dataStorage, seriesIDs, series)
	runtime.KeepAlive(ds)
}

func (ds *DataStorage) QueryFinal(queriers []uintptr) {
	seriesDataDataStorageQueryFinal(queriers)
	runtime.KeepAlive(queriers)
}
