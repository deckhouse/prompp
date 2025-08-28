package cppbridge_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/stretchr/testify/suite"
)

type HeadSuite struct {
	suite.Suite
	lss         *cppbridge.LabelSetStorage
	dataStorage *cppbridge.HeadDataStorage
	encoder     *cppbridge.HeadEncoder
}

func TestHeadSuite(t *testing.T) {
	suite.Run(t, new(HeadSuite))
}

func (s *HeadSuite) SetupTest() {
	s.lss = cppbridge.NewQueryableLssStorage()
	s.dataStorage = cppbridge.NewHeadDataStorage()
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
	recoder := cppbridge.NewChunkRecoder(s.lss, 2, s.dataStorage, cppbridge.TimeInterval{MinT: 0, MaxT: 4})

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

	recoder := cppbridge.NewChunkRecoder(s.lss, 1, s.dataStorage, cppbridge.TimeInterval{MinT: 0, MaxT: 4})

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
	serializedChunks, result := s.dataStorage.Query(cppbridge.HeadDataStorageQuery{
		StartTimestampMs: timeInterval.MinT,
		EndTimestampMs:   timeInterval.MaxT,
		LabelSetIDs:      []uint32{0, 1}},
	)
	recoder := cppbridge.NewSerializedChunkRecoder(serializedChunks.CBytes(), timeInterval)

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
	dataStorage := cppbridge.NewHeadDataStorage()
	encoder := cppbridge.NewHeadEncoderWithDataStorage(dataStorage)
	encoder.Encode(0, 1, 1.0)
	encoder.Encode(0, 2, 1.0)
	encoder.Encode(1, 2, 1.0)
	encoder.Encode(1, 3, 1.0)

	// Act
	timeInterval := dataStorage.TimeInterval()

	// Assert
	s.Equal(cppbridge.TimeInterval{MinT: 1, MaxT: 3}, timeInterval)
}

func (s *HeadSuite) TestInstantQuery() {
	// Arrange
	dataStorage := cppbridge.NewHeadDataStorage()
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
	// Act

	samples, result := dataStorage.InstantQuery(targetTimestamp, defaultTimestamp, seriesIDs)

	// Assert
	require.Equal(s.T(), cppbridge.DataStorageQueryStatusSuccess, result.Status)
	require.Len(s.T(), samples, 4)

	s.Equal(defaultTimestamp, samples[0].Timestamp)
	s.Equal(series[2].Sample, samples[1])
	s.Equal(series[5].Sample, samples[2])
	s.Equal(series[6].Sample, samples[3])
}
