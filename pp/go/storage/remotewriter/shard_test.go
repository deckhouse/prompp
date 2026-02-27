package remotewriter

import (
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/remotewriter/remotewritertest"
	"github.com/stretchr/testify/suite"
)

type ShardSuite struct {
	suite.Suite

	unexpectedEOFCount prometheus.Counter
	segmentSize        prometheus.Histogram
}

func TestShardSuite(t *testing.T) {
	suite.Run(t, new(ShardSuite))
}

func (s *ShardSuite) SetupTest() {
	s.unexpectedEOFCount = prometheus.NewCounter(prometheus.CounterOpts{})
	s.segmentSize = prometheus.NewHistogram(prometheus.HistogramOpts{})
}

func (s *ShardSuite) TestRead() {
	shardID := uint16(0)
	dataDir := s.T().TempDir()
	shardFilePath := filepath.Join(dataDir, fmt.Sprintf("shard_%d.wal", shardID))
	decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.state", shardID))
	numberOfSegments := uint32(10)

	err := remotewritertest.WriteToShardWalFileV1(s.T().Context(), shardFilePath, uint64(numberOfSegments))
	s.Require().NoError(err)

	shard, err := newShard(
		s.T().Name(),
		shardID,
		shardFilePath,
		decoderStateFileName,
		true,
		labels.EmptyLabels(),
		[]*cppbridge.RelabelConfig{},
		s.unexpectedEOFCount,
		s.segmentSize,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(shard.Close()) }()

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(1)

	for sid := range numberOfSegments {
		segment, readErr := shard.Read(s.T().Context(), sid, 0, segmentSampleStorages.Get(uint64(shardID)))
		s.Require().NoError(readErr)

		s.Require().Equal(sid, segment.ID)
	}

	_, err = shard.Read(s.T().Context(), numberOfSegments, 0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().ErrorIs(err, io.EOF)
	s.Require().ErrorIs(err, ErrShardIsCorrupted)
}

func (s *ShardSuite) TestSkipSegments() {
	shardID := uint16(0)
	dataDir := s.T().TempDir()
	shardFilePath := filepath.Join(dataDir, fmt.Sprintf("shard_%d.wal", shardID))
	decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.state", shardID))
	numberOfSegments := uint32(10)

	err := remotewritertest.WriteToShardWalFileV1(s.T().Context(), shardFilePath, uint64(numberOfSegments))
	s.Require().NoError(err)

	shard, err := newShard(
		s.T().Name(),
		shardID,
		shardFilePath,
		decoderStateFileName,
		true,
		labels.EmptyLabels(),
		[]*cppbridge.RelabelConfig{},
		s.unexpectedEOFCount,
		s.segmentSize,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(shard.Close()) }()

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(1)

	segment, readErr := shard.Read(s.T().Context(), numberOfSegments-1, 0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().NoError(readErr)

	s.Require().Equal(numberOfSegments-1, segment.ID)

	_, err = shard.Read(s.T().Context(), numberOfSegments, 0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().ErrorIs(err, io.EOF)
	s.Require().ErrorIs(err, ErrShardIsCorrupted)
}
