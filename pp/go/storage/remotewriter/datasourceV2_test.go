package remotewriter

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/remotewriter/remotewritertest"
)

type DataSourceActiveSuite struct {
	suite.Suite

	segmentSize prometheus.Histogram
}

func TestDataSourceActiveSuite(t *testing.T) {
	suite.Run(t, new(DataSourceActiveSuite))
}

func (s *DataSourceActiveSuite) SetupSuite() {
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
	startTimestamp := int64(0)

	err := remotewritertest.WriteToShardWalFileV1Multi(
		baseCtx,
		shardFilePaths,
		remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments)),
	)
	s.Require().NoError(err)

	discardCache := true

	clock := clockwork.NewRealClock()
	c, err := remotewritertest.MakeCatalog(dataDir, clock)
	s.Require().NoError(err)

	rec, err := c.Create(numberOfShards)
	s.Require().NoError(err)

	rec.SetLastAppendedSegmentID(numberOfSegments/2 - 1)
	corruptMarker := CorruptMarkerFn(func(string) error { return nil })

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

	_, err = c.SetStatus(rec.ID(), catalog.StatusRotated)
	s.Require().NoError(err)

	segments, err = dataSource.Next(baseCtx, 0, segmentSampleStorages)
	s.Require().ErrorIs(err, ErrEndOfBlock)
	s.Require().Empty(segments)

	runtime.KeepAlive(segmentSampleStorages)
}

func (s *DataSourceActiveSuite) TestNextV2() {
	dataDir := s.T().TempDir()
	numberOfSegments := uint32(10)
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow

	clock := clockwork.NewRealClock()
	c, err := remotewritertest.MakeCatalog(dataDir, clock)
	s.Require().NoError(err)

	rec, err := c.Create(numberOfShards)
	s.Require().NoError(err)

	baseCtx := s.T().Context()
	startTimestamp := int64(0)

	err = remotewritertest.WriteToShardWalFileV2Multi(
		baseCtx,
		shardFilePaths,
		remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments)),
		rec,
	)
	s.Require().NoError(err)

	discardCache := true
	rec.SetLastAppendedSegmentID(numberOfSegments/2 - 1)
	corruptMarker := CorruptMarkerFn(func(string) error { return nil })

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

	_, err = c.SetStatus(rec.ID(), catalog.StatusRotated)
	s.Require().NoError(err)

	segments, err = dataSource.Next(baseCtx, 0, segmentSampleStorages)
	s.Require().ErrorIs(err, ErrEndOfBlock)
	s.Require().Empty(segments)

	runtime.KeepAlive(segmentSampleStorages)
}

func (s *DataSourceActiveSuite) TestRestoreReadV1() {
	dataDir := s.T().TempDir()
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	numberOfSegments := uint32(10)
	baseCtx := s.T().Context()
	startTimestamp := int64(0)

	err := remotewritertest.WriteToShardWalFileV1Multi(
		baseCtx,
		shardFilePaths,
		remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments)),
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

	err = dataSource.Init(baseCtx, numberOfSegments/2-1)
	s.Require().NoError(err)

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(uint64(numberOfShards))

	segments, readErr := dataSource.Next(baseCtx, 0, segmentSampleStorages)
	s.Require().NoError(readErr)

	s.Require().Len(segments, 2)
	s.Require().Equal(numberOfSegments/2-1, segments[0].ID)
	s.Require().Equal(numberOfSegments/2-1, segments[1].ID)
	s.Require().Equal(int64(numberOfSegments-2), segments[0].MaxTimestamp)
	s.Require().Equal(int64(numberOfSegments-1), segments[1].MaxTimestamp)
	s.Require().Equal(uint32(1), segments[0].SampleCount)
	s.Require().Equal(uint32(0), segments[0].DroppedSamplesCount)
	s.Require().Equal(uint32(0), segments[0].DroppedSeriesCount)
	s.Require().Equal(uint32(0), segments[0].OutdatedSamplesCount)

	segments, err = dataSource.Next(baseCtx, 0, segmentSampleStorages)
	s.Require().ErrorIs(err, ErrEmptyReadResult)
	s.Require().Empty(segments)

	runtime.KeepAlive(segmentSampleStorages)
}

func (s *DataSourceActiveSuite) TestRestoreReadV2() {
	dataDir := s.T().TempDir()
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	numberOfSegments := uint32(10)
	baseCtx := s.T().Context()
	rec := remotewritertest.MakeRecord(numberOfShards)
	startTimestamp := int64(0)

	err := remotewritertest.WriteToShardWalFileV2Multi(
		baseCtx,
		shardFilePaths,
		remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments)),
		rec,
	)
	s.Require().NoError(err)

	discardCache := true
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

	runtime.KeepAlive(segmentSampleStorages)
}

func (s *DataSourceActiveSuite) TestSkipByMinTimeV1() {
	dataDir := s.T().TempDir()
	numberOfSegments := uint32(10)
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	baseCtx := s.T().Context()
	startTimestamp := int64(0)

	err := remotewritertest.WriteToShardWalFileV1Multi(
		baseCtx,
		shardFilePaths,
		remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments)),
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

	minTimestamp := int64(numberOfSegments)

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(uint64(numberOfShards))
	for sid := range numberOfSegments / 2 {
		segments, readErr := dataSource.Next(baseCtx, minTimestamp, segmentSampleStorages)
		s.Require().NoError(readErr)

		s.Require().Len(segments, 2)
		s.Require().Equal(sid, segments[0].ID)
		s.Require().Equal(sid, segments[1].ID)
		s.Require().Equal(int64(0), segments[0].MaxTimestamp)
		s.Require().Equal(uint32(0), segments[0].SampleCount)
		s.Require().Equal(uint32(0), segments[0].DroppedSamplesCount)
		s.Require().Equal(uint32(0), segments[0].DroppedSeriesCount)
		s.Require().Equal(uint32(1), segments[0].OutdatedSamplesCount)
	}

	segments, err := dataSource.Next(baseCtx, 0, segmentSampleStorages)
	s.Require().ErrorIs(err, ErrEmptyReadResult)
	s.Require().Empty(segments)

	runtime.KeepAlive(segmentSampleStorages)
}

func (s *DataSourceActiveSuite) TestSkipByMinTimeV2() {
	dataDir := s.T().TempDir()
	numberOfSegments := uint32(10)
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	baseCtx := s.T().Context()
	rec := remotewritertest.MakeRecord(numberOfShards)
	startTimestamp := int64(0)

	err := remotewritertest.WriteToShardWalFileV2Multi(
		baseCtx,
		shardFilePaths,
		remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments)),
		rec,
	)
	s.Require().NoError(err)

	discardCache := true
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

	runtime.KeepAlive(segmentSampleStorages)
}

func (s *DataSourceActiveSuite) TestFileNotExistsV1() {
	dataDir := s.T().TempDir()
	numberOfSegments := uint32(10)
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	baseCtx := s.T().Context()
	startTimestamp := int64(0)

	err := remotewritertest.WriteToShardWalFileV1Multi(
		baseCtx,
		shardFilePaths,
		remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments)),
	)
	s.Require().NoError(err)

	s.Require().NoError(os.Remove(shardFilePaths[0]))

	discardCache := true
	rec := remotewritertest.MakeRecord(numberOfShards)
	rec.SetLastAppendedSegmentID(numberOfSegments/2 - 1)
	corrupt := false
	corruptMarker := CorruptMarkerFn(func(string) error {
		corrupt = true
		return nil
	})
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
	s.Require().Error(err)
	s.Require().Nil(dataSource)
	s.Require().False(corrupt)
}

func (s *DataSourceActiveSuite) TestFileNotExistsV2() {
	dataDir := s.T().TempDir()
	numberOfSegments := uint32(10)
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	baseCtx := s.T().Context()
	rec := remotewritertest.MakeRecord(numberOfShards)
	startTimestamp := int64(0)

	err := remotewritertest.WriteToShardWalFileV2Multi(
		baseCtx,
		shardFilePaths,
		remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments)),
		rec,
	)
	s.Require().NoError(err)

	s.Require().NoError(os.Remove(shardFilePaths[0]))

	discardCache := true
	rec.SetLastAppendedSegmentID(numberOfSegments/2 - 1)
	corrupt := false
	corruptMarker := CorruptMarkerFn(func(string) error {
		corrupt = true
		return nil
	})
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
	s.Require().Error(err)
	s.Require().Nil(dataSource)
	s.Require().False(corrupt)
}

func (s *DataSourceActiveSuite) TestCorruptedShardV1() {
	dataDir := s.T().TempDir()
	numberOfSegments := uint32(10)
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	baseCtx := s.T().Context()
	startTimestamp := int64(0)

	err := remotewritertest.WriteToShardWalFileV1Multi(
		baseCtx,
		shardFilePaths,
		remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments)),
	)
	s.Require().NoError(err)

	s.Require().NoError(os.Truncate(shardFilePaths[0], 11))

	discardCache := true
	clock := clockwork.NewRealClock()
	c, err := remotewritertest.MakeCatalog(dataDir, clock)
	s.Require().NoError(err)

	rec, err := c.Create(numberOfShards)
	s.Require().NoError(err)

	rec.SetLastAppendedSegmentID(numberOfSegments/2 - 1)
	corrupt := false
	corruptMarker := CorruptMarkerFn(func(string) error {
		corrupt = true
		return nil
	})

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

		s.Require().Len(segments, 1)
		s.Require().Equal(sid, segments[0].ID)
		s.Require().Equal(int64(sid*2+1), segments[0].MaxTimestamp)
		s.Require().Equal(uint32(1), segments[0].SampleCount)
		s.Require().Equal(uint32(0), segments[0].DroppedSamplesCount)
		s.Require().Equal(uint32(0), segments[0].DroppedSeriesCount)
		s.Require().Equal(uint32(0), segments[0].OutdatedSamplesCount)
	}

	segments, err := dataSource.Next(baseCtx, 0, segmentSampleStorages)
	s.Require().ErrorIs(err, ErrEmptyReadResult)
	s.Require().Empty(segments)

	_, err = c.SetStatus(rec.ID(), catalog.StatusRotated)
	s.Require().NoError(err)

	segments, err = dataSource.Next(baseCtx, 0, segmentSampleStorages)
	s.Require().ErrorIs(err, ErrEndOfBlock)
	s.Require().Empty(segments)
	s.Require().True(corrupt)
}

func (s *DataSourceActiveSuite) TestCorruptedShardV2() {
	dataDir := s.T().TempDir()
	numberOfSegments := uint32(10)
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	baseCtx := s.T().Context()

	clock := clockwork.NewRealClock()
	c, err := remotewritertest.MakeCatalog(dataDir, clock)
	s.Require().NoError(err)

	rec, err := c.Create(numberOfShards)
	s.Require().NoError(err)

	startTimestamp := int64(0)

	err = remotewritertest.WriteToShardWalFileV2Multi(
		baseCtx,
		shardFilePaths,
		remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments)),
		rec,
	)
	s.Require().NoError(err)

	s.Require().NoError(os.Truncate(shardFilePaths[0], 11))

	discardCache := true
	rec.SetLastAppendedSegmentID(numberOfSegments/2 - 1)
	corrupt := false
	corruptMarker := CorruptMarkerFn(func(string) error {
		corrupt = true
		return nil
	})

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
		if sid%2 == 0 && sid != 0 { // 0 segmentis corrupted, but we require read it
			// skip removed shards segments
			continue
		}

		segments, readErr := dataSource.Next(baseCtx, 0, segmentSampleStorages)
		s.Require().NoError(readErr)

		if len(segments) == 0 {
			// skip corrupted shards segments
			continue
		}

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

	_, err = c.SetStatus(rec.ID(), catalog.StatusRotated)
	s.Require().NoError(err)

	segments, err = dataSource.Next(baseCtx, 0, segmentSampleStorages)
	s.Require().ErrorIs(err, ErrEndOfBlock)
	s.Require().Empty(segments)
	s.Require().True(corrupt)
}

//
// DataSourceRotatedSuite
//

type DataSourceRotatedSuite struct {
	suite.Suite

	segmentSize prometheus.Histogram
}

func TestDataSourceRotatedSuite(t *testing.T) {
	suite.Run(t, new(DataSourceRotatedSuite))
}

func (s *DataSourceRotatedSuite) SetupSuite() {
	s.segmentSize = prometheus.NewHistogram(prometheus.HistogramOpts{})
}

func (s *DataSourceRotatedSuite) TestNextV1() {
	dataDir := s.T().TempDir()
	numberOfSegments := uint32(10)
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	baseCtx := s.T().Context()
	startTimestamp := int64(0)

	err := remotewritertest.WriteToShardWalFileV1Multi(
		baseCtx,
		shardFilePaths,
		remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments)),
	)
	s.Require().NoError(err)

	discardCache := true
	rec := remotewritertest.MakeRecord(numberOfShards)
	rec.SetLastAppendedSegmentID(numberOfSegments/2 - 1)
	corruptMarker := CorruptMarkerFn(func(string) error { return nil })
	clock := clockwork.NewRealClock()
	dataSource, err := newDataSourceRotated(
		dataDir,
		DestinationConfig{},
		numberOfShards,
		discardCache,
		clock,
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
	s.Require().ErrorIs(err, ErrEndOfBlock)
	s.Require().Empty(segments)
}

func (s *DataSourceRotatedSuite) TestNextV2() {
	dataDir := s.T().TempDir()
	numberOfSegments := uint32(10)
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	baseCtx := s.T().Context()
	rec := remotewritertest.MakeRecord(numberOfShards)
	startTimestamp := int64(0)

	err := remotewritertest.WriteToShardWalFileV2Multi(
		baseCtx,
		shardFilePaths,
		remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments)),
		rec,
	)
	s.Require().NoError(err)

	discardCache := true
	rec.SetLastAppendedSegmentID(numberOfSegments/2 - 1)
	corruptMarker := CorruptMarkerFn(func(string) error { return nil })
	clock := clockwork.NewRealClock()
	dataSource, err := newDataSourceRotated(
		dataDir,
		DestinationConfig{},
		numberOfShards,
		discardCache,
		clock,
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
	s.Require().ErrorIs(err, ErrEndOfBlock)
	s.Require().Empty(segments)
}

func (s *DataSourceRotatedSuite) TestRestoreReadV1() {
	dataDir := s.T().TempDir()
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	numberOfSegments := uint32(10)
	baseCtx := s.T().Context()
	startTimestamp := int64(0)

	err := remotewritertest.WriteToShardWalFileV1Multi(
		baseCtx,
		shardFilePaths,
		remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments)),
	)
	s.Require().NoError(err)

	discardCache := true
	rec := remotewritertest.MakeRecord(numberOfShards)
	rec.SetLastAppendedSegmentID(numberOfSegments/2 - 1)
	corruptMarker := CorruptMarkerFn(func(string) error { return nil })
	clock := clockwork.NewRealClock()
	dataSource, err := newDataSourceRotated(
		dataDir,
		DestinationConfig{},
		numberOfShards,
		discardCache,
		clock,
		corruptMarker,
		rec,
		s.segmentSize,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(dataSource.Close()) }()

	err = dataSource.Init(baseCtx, numberOfSegments/2-1)
	s.Require().NoError(err)

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(uint64(numberOfShards))

	segments, readErr := dataSource.Next(baseCtx, 0, segmentSampleStorages)
	s.Require().NoError(readErr)

	s.Require().Len(segments, 2)
	s.Require().Equal(numberOfSegments/2-1, segments[0].ID)
	s.Require().Equal(numberOfSegments/2-1, segments[1].ID)
	s.Require().Equal(int64(numberOfSegments-2), segments[0].MaxTimestamp)
	s.Require().Equal(int64(numberOfSegments-1), segments[1].MaxTimestamp)
	s.Require().Equal(uint32(1), segments[0].SampleCount)
	s.Require().Equal(uint32(0), segments[0].DroppedSamplesCount)
	s.Require().Equal(uint32(0), segments[0].DroppedSeriesCount)
	s.Require().Equal(uint32(0), segments[0].OutdatedSamplesCount)

	segments, err = dataSource.Next(baseCtx, 0, segmentSampleStorages)
	s.Require().ErrorIs(err, ErrEndOfBlock)
	s.Require().Empty(segments)
}

func (s *DataSourceRotatedSuite) TestRestoreReadV2() {
	dataDir := s.T().TempDir()
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	numberOfSegments := uint32(10)
	baseCtx := s.T().Context()
	rec := remotewritertest.MakeRecord(numberOfShards)
	startTimestamp := int64(0)

	err := remotewritertest.WriteToShardWalFileV2Multi(
		baseCtx,
		shardFilePaths,
		remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments)),
		rec,
	)
	s.Require().NoError(err)

	discardCache := true
	rec.SetLastAppendedSegmentID(numberOfSegments/2 - 1)
	corruptMarker := CorruptMarkerFn(func(string) error { return nil })
	clock := clockwork.NewRealClock()
	dataSource, err := newDataSourceRotated(
		dataDir,
		DestinationConfig{},
		numberOfShards,
		discardCache,
		clock,
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
	s.Require().ErrorIs(err, ErrEndOfBlock)
	s.Require().Empty(segments)
}

func (s *DataSourceRotatedSuite) TestSkipByMinTimeV1() {
	dataDir := s.T().TempDir()
	numberOfSegments := uint32(10)
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	baseCtx := s.T().Context()
	startTimestamp := int64(0)

	err := remotewritertest.WriteToShardWalFileV1Multi(
		baseCtx,
		shardFilePaths,
		remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments)),
	)
	s.Require().NoError(err)

	discardCache := true
	rec := remotewritertest.MakeRecord(numberOfShards)
	rec.SetLastAppendedSegmentID(numberOfSegments/2 - 1)
	corruptMarker := CorruptMarkerFn(func(string) error { return nil })
	clock := clockwork.NewRealClock()
	dataSource, err := newDataSourceRotated(
		dataDir,
		DestinationConfig{},
		numberOfShards,
		discardCache,
		clock,
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
	for sid := range numberOfSegments / 2 {
		segments, readErr := dataSource.Next(baseCtx, minTimestamp, segmentSampleStorages)
		s.Require().NoError(readErr)

		s.Require().Len(segments, 2)
		s.Require().Equal(sid, segments[0].ID)
		s.Require().Equal(sid, segments[1].ID)
		s.Require().Equal(int64(0), segments[0].MaxTimestamp)
		s.Require().Equal(uint32(0), segments[0].SampleCount)
		s.Require().Equal(uint32(0), segments[0].DroppedSamplesCount)
		s.Require().Equal(uint32(0), segments[0].DroppedSeriesCount)
		s.Require().Equal(uint32(1), segments[0].OutdatedSamplesCount)
	}

	segments, err := dataSource.Next(baseCtx, 0, segmentSampleStorages)
	s.Require().ErrorIs(err, ErrEndOfBlock)
	s.Require().Empty(segments)
}

func (s *DataSourceRotatedSuite) TestSkipByMinTimeV2() {
	dataDir := s.T().TempDir()
	numberOfSegments := uint32(10)
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	baseCtx := s.T().Context()
	rec := remotewritertest.MakeRecord(numberOfShards)
	startTimestamp := int64(0)

	err := remotewritertest.WriteToShardWalFileV2Multi(
		baseCtx,
		shardFilePaths,
		remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments)),
		rec,
	)
	s.Require().NoError(err)

	discardCache := true
	rec.SetLastAppendedSegmentID(numberOfSegments/2 - 1)
	corruptMarker := CorruptMarkerFn(func(string) error { return nil })
	clock := clockwork.NewRealClock()
	dataSource, err := newDataSourceRotated(
		dataDir,
		DestinationConfig{},
		numberOfShards,
		discardCache,
		clock,
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
	s.Require().ErrorIs(err, ErrEndOfBlock)
	s.Require().Empty(segments)
}

func (s *DataSourceRotatedSuite) TestFileNotExistsV1() {
	dataDir := s.T().TempDir()
	numberOfSegments := uint32(10)
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	baseCtx := s.T().Context()
	startTimestamp := int64(0)

	err := remotewritertest.WriteToShardWalFileV1Multi(
		baseCtx,
		shardFilePaths,
		remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments)),
	)
	s.Require().NoError(err)

	s.Require().NoError(os.Remove(shardFilePaths[0]))

	discardCache := true
	rec := remotewritertest.MakeRecord(numberOfShards)
	rec.SetLastAppendedSegmentID(numberOfSegments/2 - 1)
	corrupt := false
	corruptMarker := CorruptMarkerFn(func(string) error {
		corrupt = true
		return nil
	})
	clock := clockwork.NewRealClock()
	dataSource, err := newDataSourceRotated(
		dataDir,
		DestinationConfig{},
		numberOfShards,
		discardCache,
		clock,
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

		s.Require().Len(segments, 1)
		s.Require().Equal(sid, segments[0].ID)
		s.Require().Equal(int64(sid*2+1), segments[0].MaxTimestamp)
		s.Require().Equal(uint32(1), segments[0].SampleCount)
		s.Require().Equal(uint32(0), segments[0].DroppedSamplesCount)
		s.Require().Equal(uint32(0), segments[0].DroppedSeriesCount)
		s.Require().Equal(uint32(0), segments[0].OutdatedSamplesCount)
	}

	segments, err := dataSource.Next(baseCtx, 0, segmentSampleStorages)
	s.Require().ErrorIs(err, ErrEndOfBlock)
	s.Require().Empty(segments)
	s.Require().True(corrupt)
}

func (s *DataSourceRotatedSuite) TestFileNotExistsV2() {
	dataDir := s.T().TempDir()
	numberOfSegments := uint32(10)
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	baseCtx := s.T().Context()
	rec := remotewritertest.MakeRecord(numberOfShards)
	startTimestamp := int64(0)

	err := remotewritertest.WriteToShardWalFileV2Multi(
		baseCtx,
		shardFilePaths,
		remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments)),
		rec,
	)
	s.Require().NoError(err)

	s.Require().NoError(os.Remove(shardFilePaths[0]))

	discardCache := true
	rec.SetLastAppendedSegmentID(numberOfSegments/2 - 1)
	corrupt := false
	corruptMarker := CorruptMarkerFn(func(string) error {
		corrupt = true
		return nil
	})
	clock := clockwork.NewRealClock()
	dataSource, err := newDataSourceRotated(
		dataDir,
		DestinationConfig{},
		numberOfShards,
		discardCache,
		clock,
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
		if sid%2 == 0 {
			// skip removed shards segments
			continue
		}

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
	s.Require().ErrorIs(err, ErrEndOfBlock)
	s.Require().Empty(segments)
	s.Require().True(corrupt)
}

func (s *DataSourceRotatedSuite) TestCorruptedShardV1() {
	dataDir := s.T().TempDir()
	numberOfSegments := uint32(10)
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	baseCtx := s.T().Context()
	startTimestamp := int64(0)

	err := remotewritertest.WriteToShardWalFileV1Multi(
		baseCtx,
		shardFilePaths,
		remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments)),
	)
	s.Require().NoError(err)

	s.Require().NoError(os.Truncate(shardFilePaths[0], 11))

	discardCache := true
	rec := remotewritertest.MakeRecord(numberOfShards)
	rec.SetLastAppendedSegmentID(numberOfSegments/2 - 1)
	corrupt := false
	corruptMarker := CorruptMarkerFn(func(string) error {
		corrupt = true
		return nil
	})
	clock := clockwork.NewRealClock()
	dataSource, err := newDataSourceRotated(
		dataDir,
		DestinationConfig{},
		numberOfShards,
		discardCache,
		clock,
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

		s.Require().Len(segments, 1)
		s.Require().Equal(sid, segments[0].ID)
		s.Require().Equal(int64(sid*2+1), segments[0].MaxTimestamp)
		s.Require().Equal(uint32(1), segments[0].SampleCount)
		s.Require().Equal(uint32(0), segments[0].DroppedSamplesCount)
		s.Require().Equal(uint32(0), segments[0].DroppedSeriesCount)
		s.Require().Equal(uint32(0), segments[0].OutdatedSamplesCount)
	}

	segments, err := dataSource.Next(baseCtx, 0, segmentSampleStorages)
	s.Require().ErrorIs(err, ErrEndOfBlock)
	s.Require().Empty(segments)
	s.Require().True(corrupt)
}

func (s *DataSourceRotatedSuite) TestCorruptedShardV2() {
	dataDir := s.T().TempDir()
	numberOfSegments := uint32(10)
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	baseCtx := s.T().Context()
	rec := remotewritertest.MakeRecord(numberOfShards)
	startTimestamp := int64(0)

	err := remotewritertest.WriteToShardWalFileV2Multi(
		baseCtx,
		shardFilePaths,
		remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments)),
		rec,
	)
	s.Require().NoError(err)

	s.Require().NoError(os.Truncate(shardFilePaths[0], 11))

	discardCache := true
	rec.SetLastAppendedSegmentID(numberOfSegments/2 - 1)
	corrupt := false
	corruptMarker := CorruptMarkerFn(func(string) error {
		corrupt = true
		return nil
	})
	clock := clockwork.NewRealClock()
	dataSource, err := newDataSourceRotated(
		dataDir,
		DestinationConfig{},
		numberOfShards,
		discardCache,
		clock,
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
		if sid%2 == 0 && sid != 0 { // 0 segmentis corrupted, but we require read it
			// skip removed shards segments
			continue
		}

		segments, readErr := dataSource.Next(baseCtx, 0, segmentSampleStorages)
		s.Require().NoError(readErr)

		if len(segments) == 0 {
			// skip corrupted shards segments
			continue
		}

		s.Require().Len(segments, 1)
		s.Require().Equal(sid, segments[0].ID)
		s.Require().Equal(int64(sid), segments[0].MaxTimestamp)
		s.Require().Equal(uint32(1), segments[0].SampleCount)
		s.Require().Equal(uint32(0), segments[0].DroppedSamplesCount)
		s.Require().Equal(uint32(0), segments[0].DroppedSeriesCount)
		s.Require().Equal(uint32(0), segments[0].OutdatedSamplesCount)
	}

	segments, err := dataSource.Next(baseCtx, 0, segmentSampleStorages)
	s.Require().ErrorIs(err, ErrEndOfBlock)
	s.Require().Empty(segments)
	s.Require().True(corrupt)
}
