package cppbridge_test

import (
	"bytes"
	"fmt"
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
	recoder := cppbridge.NewChunkRecoder(s.lss, s.dataStorage, cppbridge.TimeInterval{MinT: 0, MaxT: 4})

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

func (s *HeadSuite) TestSerializedChunkRecoder() {
	// Arrange
	s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("job", "1").Build())
	s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("job", "2").Build())

	s.encoder.Encode(0, 1, 1.0)
	s.encoder.Encode(0, 2, 1.0)
	s.encoder.Encode(1, 3, 2.0)
	s.encoder.Encode(1, 4, 2.0)

	timeInterval := cppbridge.TimeInterval{MinT: 0, MaxT: 4}
	serializedChunks := s.dataStorage.Query(cppbridge.HeadDataStorageQuery{
		StartTimestampMs: timeInterval.MinT,
		EndTimestampMs:   timeInterval.MaxT,
		LabelSetIDs:      []uint32{0, 1}},
	)
	recoder := cppbridge.NewSerializedChunkRecoder(serializedChunks, timeInterval)

	// Act
	chunk1 := recoder.RecodeNextChunk()
	chunk1.ChunkData = append([]byte(nil), chunk1.ChunkData...)
	chunk2 := recoder.RecodeNextChunk()

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

	samples := dataStorage.InstantQuery(targetTimestamp, defaultTimestamp, seriesIDs)

	// Assert
	require.Len(s.T(), samples, 4)

	s.Equal(defaultTimestamp, samples[0].Timestamp)
	s.Equal(series[2].Sample, samples[1])
	s.Equal(series[5].Sample, samples[2])
	s.Equal(series[6].Sample, samples[3])
}

type BufferReaderAtWriterCloser struct {
	buffer []byte
}

func (s *BufferReaderAtWriterCloser) ReadAt(p []byte, off int64) (n int, err error) {
	return bytes.NewReader(s.buffer).ReadAt(p, off)
}

func (s *BufferReaderAtWriterCloser) Write(p []byte) (n int, err error) {
	s.buffer = append(s.buffer, p...)
	return len(p), nil
}

func (s *BufferReaderAtWriterCloser) Close() error {
	return nil
}

type UnloadedDataStorageSuite struct {
	suite.Suite
	storageBuffer *BufferReaderAtWriterCloser
	storage       *cppbridge.UnloadedDataStorage
}

func TestUnloadedDataStorageSuite(t *testing.T) {
	suite.Run(t, new(UnloadedDataStorageSuite))
}

func (s *UnloadedDataStorageSuite) SetupTest() {
	s.storageBuffer = &BufferReaderAtWriterCloser{}
	s.storage = cppbridge.NewUnloadedDataStorage(s.storageBuffer)
}

func (s *UnloadedDataStorageSuite) readSnapshots() (string, error) {
	var snapshots string
	err := s.storage.ForEachShard(func(snapshot []byte) {
		snapshots += string(snapshot)
	})
	return snapshots, err
}

func (s *UnloadedDataStorageSuite) TestReadEmptySnapshots() {
	// Arrange

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Equal("", snapshots)
	s.Equal(nil, err)
}

func (s *UnloadedDataStorageSuite) TestReadOneSnapshot() {
	// Arrange
	_ = s.storage.Write([]byte("12345"))

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Equal("12345", snapshots)
	s.Equal(nil, err)
}

func (s *UnloadedDataStorageSuite) TestReadMultipleSnapshots() {
	// Arrange
	_ = s.storage.Write([]byte("123"))
	_ = s.storage.Write([]byte("45678"))
	_ = s.storage.Write([]byte("90"))

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Equal("1234567890", snapshots)
	s.Equal(nil, err)
}

func (s *UnloadedDataStorageSuite) TestReadEof() {
	// Arrange
	_ = s.storage.Write([]byte("123"))
	s.storageBuffer.buffer = s.storageBuffer.buffer[:len(s.storageBuffer.buffer)-1]

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Equal("", snapshots)
	s.Equal(fmt.Errorf("EOF"), err)
}

func (s *UnloadedDataStorageSuite) TestReadInvalidSnapshot() {
	// Arrange
	_ = s.storage.Write([]byte("123"))
	_ = s.storage.Write([]byte("45678"))
	s.storageBuffer.buffer[3] = 0x00

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Equal("123", snapshots)
	s.Equal(fmt.Errorf("invalid snapshot at index 1"), err)
}
