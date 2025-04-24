package cppbridge_test

import (
	"github.com/stretchr/testify/require"
	"testing"

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
	recoder := cppbridge.NewChunkRecoder(s.lss, s.dataStorage, cppbridge.TimeInterval{MinT: 0, MaxT: 5})

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

func (s *HeadSuite) TestTimeInterval() {
	// Arrange
	dataStorage := cppbridge.NewHeadDataStorage()
	encoder := cppbridge.NewHeadEncoderWithDataStorage(dataStorage)
	encoder.Encode(0, 1, 1.0)
	encoder.Encode(0, 2, 1.0)
	encoder.Encode(1, 2, 1.0)
	encoder.Encode(1, 3, 1.0)

	// Act
	time_interval := dataStorage.TimeInterval()

	// Assert
	s.Equal(cppbridge.TimeInterval{MinT: 1, MaxT: 3}, time_interval)
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

	samples := dataStorage.InstantQuery(targetTimestamp, defaultTimestamp, seriesIDs)

	// Assert
	require.Len(s.T(), samples, 4)

	s.Equal(defaultTimestamp, samples[0].Timestamp)
	s.Equal(series[2].Sample, samples[1])
	s.Equal(series[5].Sample, samples[2])
	s.Equal(series[6].Sample, samples[3])
}
