package shard

import "github.com/prometheus/prometheus/pp/go/cppbridge"

// DataStorage samles storage with labels IDs.
type DataStorage struct {
	dataStorage *cppbridge.HeadDataStorage
	encoder     *cppbridge.HeadEncoder
}

// NewDataStorage int new [DataStorage].
func NewDataStorage() *DataStorage {
	dataStorage := cppbridge.NewHeadDataStorage()
	return &DataStorage{
		dataStorage: dataStorage,
		encoder:     cppbridge.NewHeadEncoderWithDataStorage(dataStorage),
	}
}

// AllocatedMemory return size of allocated memory for DataStorage.
func (ds *DataStorage) AllocatedMemory() uint64 {
	return ds.dataStorage.AllocatedMemory()
}

// AppendInnerSeriesSlice add InnerSeries to storage.
func (ds *DataStorage) AppendInnerSeriesSlice(innerSeriesSlice []*cppbridge.InnerSeries) {
	ds.encoder.EncodeInnerSeriesSlice(innerSeriesSlice)
}

// InstantQuery make instant query to data storage and returns serialazed chunks.
func (ds *DataStorage) InstantQuery(
	targetTimestamp, notFoundValueTimestampValue int64, seriesIDs []uint32,
) []cppbridge.Sample {
	return ds.dataStorage.InstantQuery(targetTimestamp, notFoundValueTimestampValue, seriesIDs)
}

// MergeOutOfOrderChunks merge chunks with out of order data chunks.
func (ds *DataStorage) MergeOutOfOrderChunks() {
	ds.encoder.MergeOutOfOrderChunks()
}

// Query make query to data storage and returns serialazed chunks.
func (ds *DataStorage) Query(query cppbridge.HeadDataStorageQuery) *cppbridge.HeadDataStorageSerializedChunks {
	return ds.dataStorage.Query(query)
}

// Raw returns raw [cppbridge.HeadDataStorage].
func (ds *DataStorage) Raw() *cppbridge.HeadDataStorage {
	return ds.dataStorage
}
