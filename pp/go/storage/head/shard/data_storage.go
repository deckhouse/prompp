package shard

import (
	"sync"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

// DataStorage samles storage with labels IDs.
type DataStorage struct {
	dataStorage *cppbridge.DataStorage
	encoder     *cppbridge.HeadEncoder
	locker      sync.RWMutex
}

// NewDataStorage int new [DataStorage].
func NewDataStorage() *DataStorage {
	dataStorage := cppbridge.NewDataStorage()
	return &DataStorage{
		dataStorage: dataStorage,
		encoder:     cppbridge.NewHeadEncoderWithDataStorage(dataStorage),
		locker:      sync.RWMutex{},
	}
}

// AllocatedMemory return size of allocated memory for [DataStorage].
func (ds *DataStorage) AllocatedMemory() uint64 {
	ds.locker.RLock()
	am := ds.dataStorage.AllocatedMemory()
	ds.locker.RUnlock()

	return am
}

// AppendInnerSeriesSlice add InnerSeries to storage.
func (ds *DataStorage) AppendInnerSeriesSlice(innerSeriesSlice []cppbridge.InnerSeries) {
	ds.locker.Lock()
	ds.encoder.EncodeInnerSeriesSlice(innerSeriesSlice)
	ds.locker.Unlock()
}

// DecodeSegment decode segment data from decoder [cppbridge.HeadWalDecoder]
// and add to encoder [cppbridge.HeadEncoder], returns createTs, encodeTs.
//
//revive:disable-next-line:confusing-results // returns createTs, encodeTs
//nolint:gocritic // returns createTs, encodeTs
func (ds *DataStorage) DecodeSegment(decoder *cppbridge.HeadWalDecoder, data []byte) (int64, int64, error) {
	return decoder.DecodeToDataStorage(data, ds.encoder)
}

// InstantQuery make instant query to data storage and fills samples in instant series.
func (ds *DataStorage) InstantQuery(
	targetTimestamp int64,
	ids []uint32,
	samples uintptr,
) cppbridge.DataStorageQueryResult {
	ds.locker.RLock()
	res := ds.dataStorage.InstantQuery(targetTimestamp, ids, samples)
	ds.locker.RUnlock()
	return res
}

func (ds *DataStorage) Encode(seriesID uint32, timestamp int64, value float64) {
	ds.locker.Lock()
	ds.encoder.Encode(seriesID, timestamp, value)
	ds.locker.Unlock()
}

// MergeOutOfOrderChunks merge chunks with out of order data chunks.
func (ds *DataStorage) MergeOutOfOrderChunks() {
	ds.locker.Lock()
	ds.encoder.MergeOutOfOrderChunks()
	ds.locker.Unlock()
}

func (ds *DataStorage) Query(query cppbridge.DataStorageQuery) cppbridge.DataStorageQueryResult {
	ds.locker.RLock()
	result := ds.dataStorage.Query(query)
	ds.locker.RUnlock()
	return result
}

// QueryFinal finishes all the queries after data load.
func (ds *DataStorage) QueryFinal(queriers []uintptr) {
	ds.locker.RLock()
	ds.dataStorage.QueryFinal(queriers)
	ds.locker.RUnlock()
}

// QueryFirstTimestamps fills timestamps with the first sample timestamp (Prometheus ms) for each series in seriesIDs.
func (ds *DataStorage) QueryFirstTimestamps(ids []uint32, timestamps []int64) {
	ds.locker.RLock()
	ds.dataStorage.QueryFirstTimestamps(ids, timestamps)
	ds.locker.RUnlock()
}

// QueryStatus get head status from [DataStorage].
func (ds *DataStorage) QueryStatus(status *cppbridge.HeadStatus) {
	ds.locker.RLock()
	status.FromDataStorage(ds.dataStorage)
	ds.locker.RUnlock()
}

// Raw returns raw [cppbridge.DataStorage].
func (ds *DataStorage) Raw() *cppbridge.DataStorage {
	return ds.dataStorage
}

// WithLock calls fn on raw [cppbridge.DataStorage] with write lock.
func (ds *DataStorage) WithLock(fn func(ds *cppbridge.DataStorage) error) error {
	ds.locker.Lock()
	err := fn(ds.dataStorage)
	ds.locker.Unlock()

	return err
}

// WithRLock calls fn on raw [cppbridge.DataStorage] with read lock.
func (ds *DataStorage) WithRLock(fn func(ds *cppbridge.DataStorage) error) error {
	ds.locker.RLock()
	err := fn(ds.dataStorage)
	ds.locker.RUnlock()

	return err
}

// TimeInterval get time interval from [DataStorage].
func (ds *DataStorage) TimeInterval(invalidateCache bool) cppbridge.TimeInterval {
	ds.locker.RLock()
	result := ds.dataStorage.TimeInterval(invalidateCache)
	ds.locker.RUnlock()

	return result
}

// CreateUnusedSeriesDataUnloader create unused series data unloader
func (ds *DataStorage) CreateUnusedSeriesDataUnloader() *cppbridge.UnusedSeriesDataUnloader {
	return ds.dataStorage.CreateUnusedSeriesDataUnloader()
}

// CreateLoader create series data unloader
func (ds *DataStorage) CreateLoader(queriers []uintptr) *cppbridge.UnloadedDataLoader {
	return ds.dataStorage.CreateLoader(queriers)
}

// CreateRevertableLoader create series data revertable unloader
func (ds *DataStorage) CreateRevertableLoader(
	lss *cppbridge.LabelSetStorage,
	lsIdBatchSize uint32,
) *cppbridge.UnloadedDataRevertableLoader {
	return ds.dataStorage.CreateRevertableLoader(lss, lsIdBatchSize)
}

// GetQueriedSeriesBitset gets the queried series bitset memory.
func (ds *DataStorage) GetQueriedSeriesBitset() []byte {
	return ds.dataStorage.GetQueriedSeriesBitset()
}

// SetQueriedSeriesBitset sets the queried series bitset.
func (ds *DataStorage) SetQueriedSeriesBitset(bitset []byte) bool {
	return ds.dataStorage.SetQueriedSeriesBitset(bitset)
}
