package shard

import (
	"sync"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

// DataStorage samles storage with labels IDs.
type DataStorage struct {
	dataStorage *cppbridge.HeadDataStorage
	encoder     *cppbridge.HeadEncoder
	locker      sync.RWMutex
}

// NewDataStorage int new [DataStorage].
func NewDataStorage() *DataStorage {
	dataStorage := cppbridge.NewHeadDataStorage()
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
func (ds *DataStorage) AppendInnerSeriesSlice(innerSeriesSlice []*cppbridge.InnerSeries) {
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

// InstantQuery make instant query to data storage and returns samples.
func (ds *DataStorage) InstantQuery(
	targetTimestamp, notFoundValueTimestampValue int64,
	seriesIDs []uint32,
) ([]cppbridge.Sample, cppbridge.DataStorageQueryResult) {
	ds.locker.RLock()
	samples, res := ds.dataStorage.InstantQuery(targetTimestamp, notFoundValueTimestampValue, seriesIDs)
	ds.locker.RUnlock()

	return samples, res
}

// MergeOutOfOrderChunks merge chunks with out of order data chunks.
func (ds *DataStorage) MergeOutOfOrderChunks() {
	ds.locker.Lock()
	ds.encoder.MergeOutOfOrderChunks()
	ds.locker.Unlock()
}

// Query make query to data storage and returns serialazed chunks.
func (ds *DataStorage) Query(
	query cppbridge.HeadDataStorageQuery,
) (*cppbridge.HeadDataStorageSerializedChunks, cppbridge.DataStorageQueryResult) {
	ds.locker.RLock()
	serializedChunks, res := ds.dataStorage.Query(query)
	ds.locker.RUnlock()

	return serializedChunks, res
}

// QueryStatus get head status from [DataStorage].
func (ds *DataStorage) QueryStatus(status *cppbridge.HeadStatus) {
	ds.locker.RLock()
	status.FromDataStorage(ds.dataStorage)
	ds.locker.RUnlock()
}

// Raw returns raw [cppbridge.HeadDataStorage].
func (ds *DataStorage) Raw() *cppbridge.HeadDataStorage {
	return ds.dataStorage
}

// WithLock calls fn on raw [cppbridge.HeadDataStorage] with write lock.
func (ds *DataStorage) WithLock(fn func(ds *cppbridge.HeadDataStorage) error) error {
	ds.locker.Lock()
	err := fn(ds.dataStorage)
	ds.locker.Unlock()

	return err
}

// WithRLock calls fn on raw [cppbridge.HeadDataStorage] with read lock.
func (ds *DataStorage) WithRLock(fn func(ds *cppbridge.HeadDataStorage) error) error {
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
