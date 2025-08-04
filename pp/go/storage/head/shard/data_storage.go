package shard

import "github.com/prometheus/prometheus/pp/go/cppbridge"

type DataStorage struct {
	dataStorage *cppbridge.HeadDataStorage
	encoder     *cppbridge.HeadEncoder
}

func NewDataStorage() *DataStorage {
	dataStorage := cppbridge.NewHeadDataStorage()
	return &DataStorage{
		dataStorage: dataStorage,
		encoder:     cppbridge.NewHeadEncoderWithDataStorage(dataStorage),
	}
}

func (ds *DataStorage) AppendInnerSeriesSlice(innerSeriesSlice []*cppbridge.InnerSeries) {
	ds.encoder.EncodeInnerSeriesSlice(innerSeriesSlice)
}

func (ds *DataStorage) Raw() *cppbridge.HeadDataStorage {
	return ds.dataStorage
}

func (ds *DataStorage) MergeOutOfOrderChunks() {
	ds.encoder.MergeOutOfOrderChunks()
}

func (ds *DataStorage) Query(query cppbridge.HeadDataStorageQuery) *cppbridge.HeadDataStorageSerializedChunks {
	return ds.dataStorage.Query(query)
}

func (ds *DataStorage) InstantQuery(targetTimestamp, notFoundValueTimestampValue int64, seriesIDs []uint32) []cppbridge.Sample {
	return ds.dataStorage.InstantQuery(targetTimestamp, notFoundValueTimestampValue, seriesIDs)
}

func (ds *DataStorage) AllocatedMemory() uint64 {
	return ds.dataStorage.AllocatedMemory()
}
