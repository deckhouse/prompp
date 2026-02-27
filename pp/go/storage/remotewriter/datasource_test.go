package remotewriter

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/remotewriter/remotewritertest"
	"github.com/stretchr/testify/suite"
)

type DataSourceSuite struct {
	suite.Suite

	unexpectedEOFCount prometheus.Counter
	segmentSize        prometheus.Histogram
}

func TestDataSourceSuite(t *testing.T) {
	suite.Run(t, new(DataSourceSuite))
}

func (s *DataSourceSuite) SetupTest() {
	s.unexpectedEOFCount = prometheus.NewCounter(prometheus.CounterOpts{})
	s.segmentSize = prometheus.NewHistogram(prometheus.HistogramOpts{})
}

func (s *DataSourceSuite) TestRead() {
	dataDir := s.T().TempDir()
	numberOfShards := uint16(1)
	numberOfSegments := uint32(10)
	baseCtx := s.T().Context()

	err := remotewritertest.WriteToShardWalFileV1(
		baseCtx,
		filepath.Join(dataDir, fmt.Sprintf("shard_%d.wal", 0)),
		uint64(numberOfSegments),
	)
	s.Require().NoError(err)

	discardCache := true
	r := remotewritertest.MakeRecord(numberOfShards)
	r.SetLastAppendedSegmentID(numberOfSegments - 1)
	corruptMarker := CorruptMarkerFn(func(string) error { return nil })
	dataSource, err := newDataSource(
		dataDir,
		numberOfShards,
		DestinationConfig{},
		discardCache,
		newSegmentReadyChecker(r),
		corruptMarker,
		r,
		s.unexpectedEOFCount,
		s.segmentSize,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(dataSource.Close()) }()

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(uint64(numberOfShards))
	for sid := range numberOfSegments {
		segments, readErr := dataSource.Read(baseCtx, sid, 0, segmentSampleStorages)
		s.Require().NoError(readErr)

		s.Require().Len(segments, 1)
		s.Require().Equal(sid, segments[0].ID)
		s.Require().Equal(int64(sid), segments[0].MaxTimestamp)
		s.Require().Equal(sid+1, segments[0].SampleCount)
		s.Require().Equal(uint32(0), segments[0].DroppedSamplesCount)
		s.Require().Equal(uint32(0), segments[0].DroppedSeriesCount)
		s.Require().Equal(uint32(0), segments[0].OutdatedSamplesCount)
	}

	_, err = dataSource.Read(baseCtx, numberOfSegments, 0, segmentSampleStorages)
	s.Require().ErrorIs(err, ErrEmptyReadResult)
}
