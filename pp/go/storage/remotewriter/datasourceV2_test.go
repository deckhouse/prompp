package remotewriter

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/remotewriter/remotewritertest"
	"github.com/stretchr/testify/suite"
)

type DataSourceActiveSuite struct {
	suite.Suite

	segmentSize prometheus.Histogram
}

func TestDataSourceActiveSuite(t *testing.T) {
	suite.Run(t, new(DataSourceActiveSuite))
}

func (s *DataSourceActiveSuite) SetupTest() {
	s.segmentSize = prometheus.NewHistogram(prometheus.HistogramOpts{})
}

func (s *DataSourceActiveSuite) TestNextV1() {
	dataDir := s.T().TempDir()
	numberOfSegments := uint32(10)
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	baseCtx := s.T().Context()

	err := remotewritertest.WriteToShardWalFileV1Multi(
		baseCtx,
		shardFilePaths,
		uint64(numberOfSegments),
	)
	s.Require().NoError(err)

	discardCache := true
	rec := remotewritertest.MakeRecord(numberOfShards)
	rec.SetLastAppendedSegmentID(numberOfSegments/2 - 1)
	corruptMarker := CorruptMarkerFn(func(string) error { return nil })
	clock := clockwork.NewRealClock()
	dataSource, err := newDataSourceActive(
		dataDir,
		DestinationConfig{},
		numberOfShards,
		discardCache,
		clock,
		newSegmentReadyChecker(rec),
		corruptMarker,
		rec,
		s.segmentSize,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(dataSource.Close()) }()

	err = dataSource.Init(baseCtx, 0)
	s.Require().NoError(err)

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(uint64(numberOfShards))
	for sid := range numberOfSegments / 2 {
		segments, readErr := dataSource.Next(baseCtx, 0, segmentSampleStorages)
		s.Require().NoError(readErr)

		s.Require().Len(segments, 2)
		s.Require().Equal(sid, segments[0].ID)
		s.Require().Equal(sid, segments[1].ID)
		s.Require().Equal(int64(sid*2), segments[0].MaxTimestamp)
		s.Require().Equal(int64(sid*2+1), segments[1].MaxTimestamp)
		s.Require().Equal(uint32(1), segments[0].SampleCount)
		s.Require().Equal(uint32(0), segments[0].DroppedSamplesCount)
		s.Require().Equal(uint32(0), segments[0].DroppedSeriesCount)
		s.Require().Equal(uint32(0), segments[0].OutdatedSamplesCount)
	}

	segments, err := dataSource.Next(baseCtx, 0, segmentSampleStorages)
	s.Require().ErrorIs(err, ErrEmptyReadResult)
	s.Require().Empty(segments)
}

func (s *DataSourceActiveSuite) TestNextV2() {
	dataDir := s.T().TempDir()
	numberOfSegments := uint32(10)
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	baseCtx := s.T().Context()
	rec := remotewritertest.MakeRecord(numberOfShards)

	err := remotewritertest.WriteToShardWalFileV2Multi(
		baseCtx,
		shardFilePaths,
		uint64(numberOfSegments),
		rec,
	)
	s.Require().NoError(err)

	discardCache := true
	rec.SetLastAppendedSegmentID(numberOfSegments - 1)
	corruptMarker := CorruptMarkerFn(func(string) error { return nil })
	clock := clockwork.NewRealClock()
	dataSource, err := newDataSourceActive(
		dataDir,
		DestinationConfig{},
		numberOfShards,
		discardCache,
		clock,
		newSegmentReadyChecker(rec),
		corruptMarker,
		rec,
		s.segmentSize,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(dataSource.Close()) }()

	err = dataSource.Init(baseCtx, 0)
	s.Require().NoError(err)

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(uint64(numberOfShards))
	for sid := range numberOfSegments {
		segments, readErr := dataSource.Next(baseCtx, 0, segmentSampleStorages)
		s.Require().NoError(readErr)

		s.Require().Len(segments, 1)
		s.Require().Equal(sid, segments[0].ID)
		s.Require().Equal(int64(sid), segments[0].MaxTimestamp)
		s.Require().Equal(uint32(1), segments[0].SampleCount)
		s.Require().Equal(uint32(0), segments[0].DroppedSamplesCount)
		s.Require().Equal(uint32(0), segments[0].DroppedSeriesCount)
		s.Require().Equal(uint32(0), segments[0].OutdatedSamplesCount)
	}

	segments, err := dataSource.Next(baseCtx, 0, segmentSampleStorages)
	s.Require().ErrorIs(err, ErrEmptyReadResult)
	s.Require().Empty(segments)
}

func (s *DataSourceActiveSuite) TestRestoreRead() {
	dataDir := s.T().TempDir()
	numberOfShards := uint16(1)
	numberOfSegments := uint32(10)
	baseCtx := s.T().Context()

	err := remotewritertest.WriteToShardWalFileV1Single(
		baseCtx,
		filepath.Join(dataDir, fmt.Sprintf("shard_%d.wal", 0)),
		uint64(numberOfSegments),
	)
	s.Require().NoError(err)

	discardCache := true
	rec := remotewritertest.MakeRecord(numberOfShards)
	rec.SetLastAppendedSegmentID(numberOfSegments - 1)
	corruptMarker := CorruptMarkerFn(func(string) error { return nil })
	clock := clockwork.NewRealClock()
	dataSource, err := newDataSourceActive(
		dataDir,
		DestinationConfig{},
		numberOfShards,
		discardCache,
		clock,
		newSegmentReadyChecker(rec),
		corruptMarker,
		rec,
		s.segmentSize,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(dataSource.Close()) }()

	err = dataSource.Init(baseCtx, numberOfSegments-1)
	s.Require().NoError(err)

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(uint64(numberOfShards))

	segments, readErr := dataSource.Next(baseCtx, 0, segmentSampleStorages)
	s.Require().NoError(readErr)

	s.Require().Len(segments, 1)
	s.Require().Equal(numberOfSegments-1, segments[0].ID)
	s.Require().Equal(int64(numberOfSegments-1), segments[0].MaxTimestamp)
	s.Require().Equal(uint32(1), segments[0].SampleCount)
	s.Require().Equal(uint32(0), segments[0].DroppedSamplesCount)
	s.Require().Equal(uint32(0), segments[0].DroppedSeriesCount)
	s.Require().Equal(uint32(0), segments[0].OutdatedSamplesCount)

	segments, err = dataSource.Next(baseCtx, 0, segmentSampleStorages)
	s.Require().ErrorIs(err, ErrEmptyReadResult)
	s.Require().Empty(segments)
}

func (s *DataSourceActiveSuite) TestSkipByMinTime() {
	dataDir := s.T().TempDir()
	numberOfShards := uint16(1)
	numberOfSegments := uint32(10)
	baseCtx := s.T().Context()

	err := remotewritertest.WriteToShardWalFileV1Single(
		baseCtx,
		filepath.Join(dataDir, fmt.Sprintf("shard_%d.wal", 0)),
		uint64(numberOfSegments),
	)
	s.Require().NoError(err)

	discardCache := true
	rec := remotewritertest.MakeRecord(numberOfShards)
	rec.SetLastAppendedSegmentID(numberOfSegments - 1)
	corruptMarker := CorruptMarkerFn(func(string) error { return nil })
	clock := clockwork.NewRealClock()
	dataSource, err := newDataSourceActive(
		dataDir,
		DestinationConfig{},
		numberOfShards,
		discardCache,
		clock,
		newSegmentReadyChecker(rec),
		corruptMarker,
		rec,
		s.segmentSize,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(dataSource.Close()) }()

	err = dataSource.Init(baseCtx, 0)
	s.Require().NoError(err)

	minTimestamp := int64(numberOfSegments)

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(uint64(numberOfShards))
	for sid := range numberOfSegments {
		segments, readErr := dataSource.Next(baseCtx, minTimestamp, segmentSampleStorages)
		s.Require().NoError(readErr)

		s.Require().Len(segments, 1)
		s.Require().Equal(sid, segments[0].ID)
		s.Require().Equal(int64(0), segments[0].MaxTimestamp)
		s.Require().Equal(uint32(0), segments[0].SampleCount)
		s.Require().Equal(uint32(0), segments[0].DroppedSamplesCount)
		s.Require().Equal(uint32(0), segments[0].DroppedSeriesCount)
		s.Require().Equal(uint32(1), segments[0].OutdatedSamplesCount)
	}

	segments, err := dataSource.Next(baseCtx, 0, segmentSampleStorages)
	s.Require().ErrorIs(err, ErrEmptyReadResult)
	s.Require().Empty(segments)
}

//
//
//

type DataSourceRotatedSuite struct {
	suite.Suite
}

func TestDataSourceRotatedSuite(t *testing.T) {
	suite.Run(t, new(DataSourceRotatedSuite))
}

func (s *DataSourceRotatedSuite) TestHappyPath() {
	ds := &dataSourceRotated{}
	_ = ds
	s.T().Log("TODO: implement")
}
