package cppbridge_test

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/storage"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/stretchr/testify/suite"
)

type HeadSuite struct {
	suite.Suite
	lss         *cppbridge.LabelSetStorage
	dataStorage *cppbridge.DataStorage
	encoder     *cppbridge.HeadEncoder
}

func TestHeadSuite(t *testing.T) {
	suite.Run(t, new(HeadSuite))
}

func (s *HeadSuite) SetupTest() {
	s.lss = cppbridge.NewQueryableLssStorage()
	s.dataStorage = cppbridge.NewDataStorage()
	s.encoder = cppbridge.NewHeadEncoderWithDataStorage(s.dataStorage)
}

func (s *HeadSuite) TestChunkRecoder() {
	// Arrange
	s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("job", "1").Build())
	s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("job", "2").Build())

	s.encoder.Encode(0, 1, 1.0)
	s.encoder.Encode(0, 2, 1.0)
	s.encoder.Encode(1, 3, 2.0)
	s.encoder.Encode(1, 4, 2.0)
	recoder := cppbridge.NewChunkRecoder(s.lss, 2, s.dataStorage, cppbridge.TimeInterval{MinT: 0, MaxT: 4}, cppbridge.NoDownsampling)

	// Act
	chunk2 := recoder.RecodeNextChunk()
	chunk2.ChunkData = append([]byte(nil), chunk2.ChunkData...)
	chunk4 := recoder.RecodeNextChunk()

	// Assert
	s.Equal(cppbridge.RecodedChunk{
		TimeInterval: cppbridge.TimeInterval{
			MinT: 1,
			MaxT: 2,
		},
		SamplesCount: 2,
		SeriesId:     0,
		HasMoreData:  true,
		ChunkData:    []byte{0x00, 0x02, 0x02, 0x3f, 0xf0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00},
	}, chunk2)
	s.Equal(cppbridge.RecodedChunk{
		TimeInterval: cppbridge.TimeInterval{
			MinT: 3,
			MaxT: 4,
		},
		SamplesCount: 2,
		SeriesId:     1,
		HasMoreData:  false,
		ChunkData:    []byte{0x00, 0x02, 0x06, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00},
	}, chunk4)
}

func (s *HeadSuite) TestChunkRecoderWithBatchIterator() {
	// Arrange
	s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("job", "1").Build())
	s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("job", "2").Build())

	s.encoder.Encode(0, 1, 1.0)
	s.encoder.Encode(0, 2, 1.0)
	s.encoder.Encode(1, 3, 2.0)
	s.encoder.Encode(1, 4, 2.0)

	recoder := cppbridge.NewChunkRecoder(s.lss, 1, s.dataStorage, cppbridge.TimeInterval{MinT: 0, MaxT: 4}, cppbridge.NoDownsampling)

	// Act
	chunk2 := recoder.RecodeNextChunk()
	chunk2.ChunkData = append([]byte(nil), chunk2.ChunkData...)
	recoder.NextBatch()
	chunk4 := recoder.RecodeNextChunk()

	// Assert
	s.Equal(cppbridge.RecodedChunk{
		TimeInterval: cppbridge.TimeInterval{
			MinT: 1,
			MaxT: 2,
		},
		SamplesCount: 2,
		SeriesId:     0,
		HasMoreData:  false,
		ChunkData:    []byte{0x00, 0x02, 0x02, 0x3f, 0xf0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00},
	}, chunk2)
	s.Equal(cppbridge.RecodedChunk{
		TimeInterval: cppbridge.TimeInterval{
			MinT: 3,
			MaxT: 4,
		},
		SamplesCount: 2,
		SeriesId:     1,
		HasMoreData:  false,
		ChunkData:    []byte{0x00, 0x02, 0x06, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00},
	}, chunk4)
}

func (s *HeadSuite) TestSerializedChunkRecoder() {
	// Arrange
	s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("job", "1").Build())
	s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("job", "2").Build())

	s.encoder.Encode(0, 1, 1.0)
	s.encoder.Encode(0, 2, 1.0)
	s.encoder.Encode(1, 3, 2.0)
	s.encoder.Encode(1, 4, 2.0)

	timeInterval := cppbridge.TimeInterval{MinT: 0, MaxT: 4}
	result := s.dataStorage.Query(cppbridge.DataStorageQuery{
		StartTimestampMs: timeInterval.MinT,
		EndTimestampMs:   timeInterval.MaxT,
		LabelSetIDs:      []uint32{0, 1},
	}, cppbridge.NoDownsampling, unsafe.Pointer(&storage.SelectHints{}))
	recoder := cppbridge.NewSerializedChunkRecoder(result.SerializedData, timeInterval)

	// Act
	chunk1 := recoder.RecodeNextChunk()
	chunk1.ChunkData = append([]byte(nil), chunk1.ChunkData...)
	chunk2 := recoder.RecodeNextChunk()

	// Assert
	s.Equal(cppbridge.DataStorageQueryStatusSuccess, result.Status)
	s.Equal(cppbridge.RecodedChunk{
		TimeInterval: cppbridge.TimeInterval{
			MinT: 1,
			MaxT: 2,
		},
		SamplesCount: 2,
		SeriesId:     0,
		HasMoreData:  true,
		ChunkData:    []byte{0x00, 0x02, 0x02, 0x3f, 0xf0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00},
	}, chunk1)
	s.Equal(cppbridge.RecodedChunk{
		TimeInterval: cppbridge.TimeInterval{
			MinT: 3,
			MaxT: 4,
		},
		SamplesCount: 2,
		SeriesId:     1,
		HasMoreData:  false,
		ChunkData:    []byte{0x00, 0x02, 0x06, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00},
	}, chunk2)
}

func (s *HeadSuite) TestTimeInterval() {
	// Arrange
	dataStorage := cppbridge.NewDataStorage()
	encoder := cppbridge.NewHeadEncoderWithDataStorage(dataStorage)
	encoder.Encode(0, 1, 1.0)
	encoder.Encode(0, 2, 1.0)
	encoder.Encode(1, 2, 1.0)
	encoder.Encode(1, 3, 1.0)

	// Act
	timeInterval := dataStorage.TimeInterval(false)
	encoder.Encode(1, 4, 1.0)
	cachedTimeInterval := dataStorage.TimeInterval(false)
	actualTimeInterval := dataStorage.TimeInterval(true)

	// Assert
	s.Equal(cppbridge.TimeInterval{MinT: 1, MaxT: 3}, timeInterval)
	s.Equal(cppbridge.TimeInterval{MinT: 1, MaxT: 3}, cachedTimeInterval)
	s.Equal(cppbridge.TimeInterval{MinT: 1, MaxT: 4}, actualTimeInterval)
}

func (s *HeadSuite) TestInstantQuery() {
	// Arrange
	dataStorage := cppbridge.NewDataStorage()
	encoder := cppbridge.NewHeadEncoderWithDataStorage(dataStorage)
	var series = []struct {
		SeriesID uint32
		cppbridge.Sample
	}{
		{0, cppbridge.Sample{7, 1.0}},
		{0, cppbridge.Sample{10, 1.0}},
		{1, cppbridge.Sample{3, 1.0}},
		{1, cppbridge.Sample{11, 1.0}},
		{2, cppbridge.Sample{1, 2.0}},
		{2, cppbridge.Sample{4, 2.0}},
		{3, cppbridge.Sample{2, 2.0}},
		{3, cppbridge.Sample{8, 2.0}},
	}

	for _, serie := range series {
		encoder.Encode(serie.SeriesID, serie.Timestamp, serie.Value)
	}

	seriesIDs := []uint32{0, 1, 2, 3}
	targetTimestamp := int64(5)
	defaultTimestamp := int64(-1)
	instantSeries := make([]querier.InstantSeries, len(seriesIDs))
	for i := range instantSeries {
		instantSeries[i].Timestamp = defaultTimestamp
	}
	// Act

	result := dataStorage.InstantQuery(targetTimestamp, seriesIDs, uintptr(unsafe.Pointer(unsafe.SliceData(instantSeries))))

	// Assert
	require.Equal(s.T(), cppbridge.DataStorageQueryStatusSuccess, result.Status)

	s.Equal(defaultTimestamp, instantSeries[0].Timestamp)
	s.Equal(series[2].Sample, cppbridge.Sample{Timestamp: instantSeries[1].Timestamp, Value: instantSeries[1].Value})
	s.Equal(series[5].Sample, cppbridge.Sample{Timestamp: instantSeries[2].Timestamp, Value: instantSeries[2].Value})
	s.Equal(series[6].Sample, cppbridge.Sample{Timestamp: instantSeries[3].Timestamp, Value: instantSeries[3].Value})
}

func (s *HeadSuite) TestQueryFirstTimestampsWithEmptySeriesIds() {
	// Arrange

	// Act
	s.dataStorage.QueryFirstTimestamps(nil, nil)

	// Assert
}

func (s *HeadSuite) TestQueryFirstTimestamps() {
	// Arrange
	s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("job", "1").Build())
	s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("job", "2").Build())

	s.encoder.Encode(0, 5, 1.0)
	s.encoder.Encode(0, 9, 1.0)
	s.encoder.Encode(1, 2, 2.0)
	s.encoder.Encode(1, 7, 2.0)

	// Act
	timestamps := make([]int64, 2)
	s.dataStorage.QueryFirstTimestamps([]uint32{1, 0}, timestamps)

	// Assert
	s.Equal([]int64{2, 5}, timestamps)
}

func (s *HeadSuite) TestQueryFirstTimestampsInFinalizedChunk() {
	// Arrange
	s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("job", "1").Build())

	s.encoder.Encode(0, 9, 1.0)
	s.encoder.Encode(0, 5, 1.0)

	s.encoder.MergeOutOfOrderChunks()

	// Act
	timestamps := make([]int64, 1)
	s.dataStorage.QueryFirstTimestamps([]uint32{0}, timestamps)

	// Assert
	s.Equal([]int64{5}, timestamps)
}

type DataStorageSerializedDataMultiSeriesIteratorSuite struct {
	suite.Suite
	lss *cppbridge.LabelSetStorage
	ds  *cppbridge.DataStorage
	enc *cppbridge.HeadEncoder
}

func TestDataStorageSerializedDataMultiSeriesIteratorSuite(t *testing.T) {
	suite.Run(t, new(DataStorageSerializedDataMultiSeriesIteratorSuite))
}

func (s *DataStorageSerializedDataMultiSeriesIteratorSuite) SetupTest() {
	s.lss = cppbridge.NewQueryableLssStorage()
	s.ds = cppbridge.NewDataStorage()
	s.enc = cppbridge.NewHeadEncoderWithDataStorage(s.ds)

	s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("job", "a").Build())
	s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("job", "b").Build())
}

type createIteratorMethod = func(*cppbridge.DataStorageSerializedData, []uint32) cppbridge.DataStorageSerializedDataMultiSeriesIterator

var createMultiSeriesIterator = cppbridge.NewDataStorageSerializedDataMultiSeriesIterator
var createAndResetMultiSeriesIterator = func(
	serializedData *cppbridge.DataStorageSerializedData,
	seriesIDs []uint32,
) cppbridge.DataStorageSerializedDataMultiSeriesIterator {
	it := createMultiSeriesIterator(serializedData, seriesIDs)
	for it.HasData() {
		it.Next()
	}
	it.Reset(serializedData, seriesIDs)
	return it
}

func (s *DataStorageSerializedDataMultiSeriesIteratorSuite) collectSamples(
	hints storage.SelectHints,
	seriesToSerialize []uint32,
	series []uint32,
	createIterator createIteratorMethod,
) []cppbridge.Sample {
	result := s.ds.Query(cppbridge.DataStorageQuery{
		StartTimestampMs: hints.Start,
		EndTimestampMs:   hints.End,
		LabelSetIDs:      seriesToSerialize,
	}, cppbridge.NoDownsampling, unsafe.Pointer(&hints))

	it := createIterator(result.SerializedData, series)
	defer it.Close()

	out := make([]cppbridge.Sample, 0)
	for it.HasData() {
		out = append(out, cppbridge.Sample{Timestamp: it.Timestamp(), Value: it.Value()})
		it.Next()
	}

	runtime.KeepAlive(result.SerializedData)
	return out
}

func (s *DataStorageSerializedDataMultiSeriesIteratorSuite) TestSum() {
	s.testSum(createMultiSeriesIterator)
}

func (s *DataStorageSerializedDataMultiSeriesIteratorSuite) TestSumWithIteratorReset() {
	s.testSum(createAndResetMultiSeriesIterator)
}

func (s *DataStorageSerializedDataMultiSeriesIteratorSuite) testSum(method createIteratorMethod) {
	// Arrange
	s.enc.Encode(0, 50, 10.0)
	s.enc.Encode(1, 80, 20.0)
	s.enc.Encode(0, 150, 20.0)
	s.enc.Encode(1, 180, 30.0)

	// Act
	samples := s.collectSamples(storage.SelectHints{
		Start:         1,
		End:           200,
		Step:          100,
		LookbackDelta: 100,
		Func:          "sum",
	}, []uint32{0, 1}, []uint32{0, 1}, method)

	// Assert
	s.Equal([]cppbridge.Sample{
		{Timestamp: 100, Value: 30.0},
		{Timestamp: 200, Value: 50.0},
	}, samples)
}

func (s *DataStorageSerializedDataMultiSeriesIteratorSuite) TestMin() {
	// Arrange
	s.enc.Encode(0, 50, 10.0)
	s.enc.Encode(1, 130, 20.0)
	s.enc.Encode(0, 150, 30.0)
	s.enc.Encode(1, 180, 20.0)

	// Act
	samples := s.collectSamples(storage.SelectHints{
		Start:         1,
		End:           200,
		Step:          100,
		LookbackDelta: 50,
		Func:          "min",
	}, []uint32{0, 1}, []uint32{0, 1}, createMultiSeriesIterator)

	// Assert
	s.Equal([]cppbridge.Sample{
		{Timestamp: 50, Value: 10.0},
		{Timestamp: 150, Value: 20.0},
		{Timestamp: 200, Value: 20.0},
	}, samples)
}

func (s *DataStorageSerializedDataMultiSeriesIteratorSuite) TestMax() {
	// Arrange
	s.enc.Encode(0, 50, 20.0)
	s.enc.Encode(1, 80, 10.0)
	s.enc.Encode(0, 150, 20.0)
	s.enc.Encode(1, 180, 30.0)

	// Act
	samples := s.collectSamples(storage.SelectHints{
		Start:         1,
		End:           200,
		Step:          100,
		LookbackDelta: 50,
		Func:          "max",
	}, []uint32{0, 1}, []uint32{0, 1}, createMultiSeriesIterator)

	// Assert
	s.Equal([]cppbridge.Sample{
		{Timestamp: 50, Value: 20.0},
		{Timestamp: 150, Value: 20.0},
		{Timestamp: 200, Value: 30.0},
	}, samples)
}

func (s *DataStorageSerializedDataMultiSeriesIteratorSuite) TestNoSeries() {
	// Arrange
	s.enc.Encode(0, 50, 20.0)
	s.enc.Encode(1, 80, 10.0)
	s.enc.Encode(0, 150, 20.0)
	s.enc.Encode(1, 180, 30.0)
	s.enc.Encode(2, 180, 30.0)

	// Act
	samples := s.collectSamples(storage.SelectHints{
		Start: 0,
		End:   200,
		Step:  100,
		Range: 100,
		Func:  "max",
	}, []uint32{0, 1}, []uint32{2}, createMultiSeriesIterator)

	// Assert
	s.Equal([]cppbridge.Sample{}, samples)
}
