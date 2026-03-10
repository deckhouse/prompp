package remotewriter

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/reader"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer"
	"github.com/prometheus/prometheus/pp/go/storage/remotewriter/remotewritertest"
)

type ShardSuite struct {
	suite.Suite

	segmentSize prometheus.Histogram
}

func TestShardSuite(t *testing.T) {
	suite.Run(t, new(ShardSuite))
}

func (s *ShardSuite) SetupSuite() {
	s.segmentSize = prometheus.NewHistogram(prometheus.HistogramOpts{})
}

func (s *ShardSuite) TestReadV1() {
	shardID := uint16(0)
	dataDir := s.T().TempDir()
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.state", shardID))
	numberOfSegments := uint32(10)

	err := remotewritertest.WriteToShardWalFileV1Multi(
		s.T().Context(),
		shardFilePaths,
		uint64(numberOfSegments),
	)
	s.Require().NoError(err)

	shard, err := newShard(
		s.T().Name(),
		shardID,
		shardFilePaths[0],
		decoderStateFileName,
		true,
		labels.EmptyLabels(),
		[]*cppbridge.RelabelConfig{},
		s.segmentSize,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(shard.Close()) }()

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(1)

	for sid := range numberOfSegments {
		if sid%2 != uint32(shardID) {
			continue
		}

		segmentID := sid / 2
		segment, readErr := shard.Read(s.T().Context(), segmentID, 0, segmentSampleStorages.Get(uint64(shardID)))
		s.Require().NoError(readErr)

		s.Require().Equal(segmentID, segment.ID)
		s.Require().Equal(uint32(1), segment.SampleCount)
		s.Require().Equal(int64(sid), segment.MaxTimestamp)
	}

	_, err = shard.Read(s.T().Context(), numberOfSegments, 0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().ErrorIs(err, io.EOF)
	s.Require().ErrorIs(err, ErrShardIsCorrupted)
}

func (s *ShardSuite) TestReadV2() {
	shardID := uint16(0)
	dataDir := s.T().TempDir()
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.state", shardID))
	numberOfSegments := uint32(10)
	rec := remotewritertest.MakeRecord(1)

	err := remotewritertest.WriteToShardWalFileV2Multi(s.T().Context(), shardFilePaths, uint64(numberOfSegments), rec)
	s.Require().NoError(err)

	shard, err := newShard(
		s.T().Name(),
		shardID,
		shardFilePaths[0],
		decoderStateFileName,
		true,
		labels.EmptyLabels(),
		[]*cppbridge.RelabelConfig{},
		s.segmentSize,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(shard.Close()) }()

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(1)

	for sid := range numberOfSegments {
		if sid%2 != uint32(shardID) {
			continue
		}

		segment, readErr := shard.Read(s.T().Context(), sid, 0, segmentSampleStorages.Get(uint64(shardID)))
		s.Require().NoError(readErr)

		s.Require().Equal(sid, segment.ID)
		s.Require().Equal(uint32(1), segment.SampleCount)
		s.Require().Equal(int64(sid), segment.MaxTimestamp)
	}

	_, err = shard.Read(s.T().Context(), numberOfSegments, 0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().ErrorIs(err, io.EOF)
	s.Require().ErrorIs(err, ErrShardIsCorrupted)
}

func (s *ShardSuite) TestSkipSegmentsV1() {
	shardID := uint16(0)
	dataDir := s.T().TempDir()
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.state", shardID))
	numberOfSegments := uint32(10)

	err := remotewritertest.WriteToShardWalFileV1Multi(s.T().Context(), shardFilePaths, uint64(numberOfSegments))
	s.Require().NoError(err)

	shard, err := newShard(
		s.T().Name(),
		shardID,
		shardFilePaths[0],
		decoderStateFileName,
		true,
		labels.EmptyLabels(),
		[]*cppbridge.RelabelConfig{},
		s.segmentSize,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(shard.Close()) }()

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(1)

	segment, readErr := shard.Read(s.T().Context(), numberOfSegments/2-1, 0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().NoError(readErr)

	s.Require().Equal(numberOfSegments/2-1, segment.ID)

	_, err = shard.Read(s.T().Context(), numberOfSegments, 0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().ErrorIs(err, io.EOF)
	s.Require().ErrorIs(err, ErrShardIsCorrupted)
}

func (s *ShardSuite) TestSkipSegmentsV2() {
	shardID := uint16(0)
	dataDir := s.T().TempDir()
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.state", shardID))
	numberOfSegments := uint32(10)
	rec := remotewritertest.MakeRecord(1)

	err := remotewritertest.WriteToShardWalFileV2Multi(s.T().Context(), shardFilePaths, uint64(numberOfSegments), rec)
	s.Require().NoError(err)

	shard, err := newShard(
		s.T().Name(),
		shardID,
		shardFilePaths[0],
		decoderStateFileName,
		true,
		labels.EmptyLabels(),
		[]*cppbridge.RelabelConfig{},
		s.segmentSize,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(shard.Close()) }()

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(1)

	segment, readErr := shard.Read(s.T().Context(), numberOfSegments-2, 0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().NoError(readErr)

	s.Require().Equal(numberOfSegments-2, segment.ID)

	_, err = shard.Read(s.T().Context(), numberOfSegments, 0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().ErrorIs(err, io.EOF)
	s.Require().ErrorIs(err, ErrShardIsCorrupted)
}

func (s *ShardSuite) TestV1FileNotExists() {
	shardID := uint16(0)
	dataDir := s.T().TempDir()
	decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.state", shardID))

	shard, err := newShard(
		s.T().Name(),
		shardID,
		filepath.Join(dataDir, "shard_0.wal"),
		decoderStateFileName,
		true,
		labels.EmptyLabels(),
		[]*cppbridge.RelabelConfig{},
		s.segmentSize,
	)
	s.Require().Nil(shard)
	s.Require().Error(err)
}

func (s *ShardSuite) TestV2FileNotExists() {
	shardID := uint16(0)
	dataDir := s.T().TempDir()
	decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.state", shardID))

	shard, err := newShard(
		s.T().Name(),
		shardID,
		filepath.Join(dataDir, "shard_0.wal"),
		decoderStateFileName,
		true,
		labels.EmptyLabels(),
		[]*cppbridge.RelabelConfig{},
		s.segmentSize,
	)
	s.Require().Nil(shard)
	s.Require().Error(err)
}

//
// ShardRotatedSuite
//

type ShardRotatedSuite struct {
	suite.Suite

	segmentSize prometheus.Histogram
}

func TestShardRotatedSuite(t *testing.T) {
	suite.Run(t, new(ShardRotatedSuite))
}

func (s *ShardRotatedSuite) SetupSuite() {
	s.segmentSize = prometheus.NewHistogram(prometheus.HistogramOpts{})
}

func (s *ShardRotatedSuite) TestReadV1() {
	shardID := uint16(0)
	dataDir := s.T().TempDir()
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.state", shardID))
	numberOfSegments := uint32(10)

	err := remotewritertest.WriteToShardWalFileV1Multi(s.T().Context(), shardFilePaths, uint64(numberOfSegments))
	s.Require().NoError(err)

	shard, err := newShardRotated(
		s.T().Name(),
		shardID,
		shardFilePaths[0],
		decoderStateFileName,
		true,
		labels.EmptyLabels(),
		[]*cppbridge.RelabelConfig{},
		s.segmentSize,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(shard.Close()) }()

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(1)

	for sid := range numberOfSegments {
		if sid%2 != uint32(shardID) {
			continue
		}

		expSegmentID := sid / 2
		segmentID, idErr := shard.SegmentID()
		s.Require().NoError(idErr)
		s.Require().Equal(expSegmentID, segmentID)

		segmentID, idErr = shard.SegmentID()
		s.Require().NoError(idErr)
		s.Require().Equal(expSegmentID, segmentID)

		segment, readErr := shard.ReadSegment(0, segmentSampleStorages.Get(uint64(shardID)))
		s.Require().NoError(readErr)

		s.Require().Equal(expSegmentID, segment.ID)
		s.Require().Equal(uint32(1), segment.SampleCount)
		s.Require().Equal(int64(sid), segment.MaxTimestamp)
	}

	segmentID, err := shard.SegmentID()
	s.Require().Error(err)
	s.Require().ErrorIs(err, ErrEndOfBlock)
	s.Require().Equal(reader.UnknownSegmentID, segmentID)

	segment, err := shard.ReadSegment(0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().Error(err)
	s.Require().ErrorIs(err, ErrEndOfBlock)
	s.Require().Nil(segment)
}

func (s *ShardRotatedSuite) TestReadV2() {
	shardID := uint16(0)
	dataDir := s.T().TempDir()
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.state", shardID))
	numberOfSegments := uint32(10)
	rec := remotewritertest.MakeRecord(1)

	err := remotewritertest.WriteToShardWalFileV2Multi(s.T().Context(), shardFilePaths, uint64(numberOfSegments), rec)
	s.Require().NoError(err)

	shard, err := newShardRotated(
		s.T().Name(),
		shardID,
		shardFilePaths[0],
		decoderStateFileName,
		true,
		labels.EmptyLabels(),
		[]*cppbridge.RelabelConfig{},
		s.segmentSize,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(shard.Close()) }()

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(1)

	for sid := range numberOfSegments {
		if sid%2 != uint32(shardID) {
			continue
		}

		segmentID, idErr := shard.SegmentID()
		s.Require().NoError(idErr)
		s.Require().Equal(sid, segmentID)

		segmentID, idErr = shard.SegmentID()
		s.Require().NoError(idErr)
		s.Require().Equal(sid, segmentID)

		segment, readErr := shard.ReadSegment(0, segmentSampleStorages.Get(uint64(shardID)))
		s.Require().NoError(readErr)

		s.Require().Equal(sid, segment.ID)
		s.Require().Equal(uint32(1), segment.SampleCount)
		s.Require().Equal(int64(sid), segment.MaxTimestamp)
	}

	segmentID, err := shard.SegmentID()
	s.Require().Error(err)
	s.Require().ErrorIs(err, ErrEndOfBlock)
	s.Require().Equal(reader.UnknownSegmentID, segmentID)

	segment, err := shard.ReadSegment(0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().Error(err)
	s.Require().Nil(segment)
	s.Require().ErrorIs(err, ErrEndOfBlock)
}

func (s *ShardRotatedSuite) TestSkipSegmentsV1() {
	shardID := uint16(0)
	dataDir := s.T().TempDir()
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.state", shardID))
	numberOfSegments := uint32(10)

	err := remotewritertest.WriteToShardWalFileV1Multi(s.T().Context(), shardFilePaths, uint64(numberOfSegments))
	s.Require().NoError(err)

	shard, err := newShardRotated(
		s.T().Name(),
		shardID,
		shardFilePaths[0],
		decoderStateFileName,
		true,
		labels.EmptyLabels(),
		[]*cppbridge.RelabelConfig{},
		s.segmentSize,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(shard.Close()) }()

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(1)

	for sid := range numberOfSegments - 2 {
		if sid%2 != uint32(shardID) {
			continue
		}

		expSegmentID := sid / 2
		segmentID, idErr := shard.SegmentID()
		s.Require().NoError(idErr)
		s.Require().Equal(expSegmentID, segmentID)

		readErr := shard.SkipSegment(0, segmentSampleStorages.Get(uint64(shardID)))
		s.Require().NoError(readErr)
	}

	segmentID, idErr := shard.SegmentID()
	s.Require().NoError(idErr)
	s.Require().Equal(numberOfSegments/2-1, segmentID)

	segment, readErr := shard.ReadSegment(0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().NoError(readErr)

	s.Require().Equal(numberOfSegments/2-1, segment.ID)
	s.Require().Equal(uint32(1), segment.SampleCount)
	s.Require().Equal(int64(numberOfSegments-2), segment.MaxTimestamp)

	segmentID, err = shard.SegmentID()
	s.Require().Error(err)
	s.Require().ErrorIs(err, ErrEndOfBlock)
	s.Require().Equal(reader.UnknownSegmentID, segmentID)

	segment, err = shard.ReadSegment(0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().Error(err)
	s.Require().ErrorIs(err, ErrEndOfBlock)
	s.Require().Nil(segment)
}

func (s *ShardRotatedSuite) TestSkipSegmentsV2() {
	shardID := uint16(0)
	dataDir := s.T().TempDir()
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.state", shardID))
	numberOfSegments := uint32(10)
	rec := remotewritertest.MakeRecord(1)

	err := remotewritertest.WriteToShardWalFileV2Multi(s.T().Context(), shardFilePaths, uint64(numberOfSegments), rec)
	s.Require().NoError(err)

	shard, err := newShardRotated(
		s.T().Name(),
		shardID,
		shardFilePaths[0],
		decoderStateFileName,
		true,
		labels.EmptyLabels(),
		[]*cppbridge.RelabelConfig{},
		s.segmentSize,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(shard.Close()) }()

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(1)

	for sid := range numberOfSegments - 2 {
		if sid%2 != uint32(shardID) {
			continue
		}

		segmentID, idErr := shard.SegmentID()
		s.Require().NoError(idErr)
		s.Require().Equal(sid, segmentID)

		readErr := shard.SkipSegment(0, segmentSampleStorages.Get(uint64(shardID)))
		s.Require().NoError(readErr)
	}

	segmentID, idErr := shard.SegmentID()
	s.Require().NoError(idErr)
	s.Require().Equal(numberOfSegments-2, segmentID)

	segment, readErr := shard.ReadSegment(0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().NoError(readErr)

	s.Require().Equal(numberOfSegments-2, segment.ID)
	s.Require().Equal(uint32(1), segment.SampleCount)
	s.Require().Equal(int64(numberOfSegments-2), segment.MaxTimestamp)

	segmentID, err = shard.SegmentID()
	s.Require().Error(err)
	s.Require().ErrorIs(err, ErrEndOfBlock)
	s.Require().Equal(reader.UnknownSegmentID, segmentID)

	segment, err = shard.ReadSegment(0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().Error(err)
	s.Require().Nil(segment)
	s.Require().ErrorIs(err, ErrEndOfBlock)
}

func (s *ShardRotatedSuite) TestFileNotExists() {
	shardID := uint16(0)
	dataDir := s.T().TempDir()
	decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.state", shardID))

	shard, err := newShardRotated(
		s.T().Name(),
		shardID,
		filepath.Join(dataDir, "shard_0.wal"),
		decoderStateFileName,
		true,
		labels.EmptyLabels(),
		[]*cppbridge.RelabelConfig{},
		s.segmentSize,
	)
	s.Require().NotNil(shard)
	s.Require().NoError(err)
	s.Require().True(shard.IsCorrupted())
	defer func() { s.Require().NoError(shard.Close()) }()

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(1)

	segmentID, idErr := shard.SegmentID()
	s.Require().Error(idErr)
	s.Require().ErrorIs(idErr, ErrShardIsCorrupted)
	s.Require().Equal(reader.UnknownSegmentID, segmentID)

	segment, err := shard.ReadSegment(0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().Error(err)
	s.Require().ErrorIs(err, ErrShardIsCorrupted)
	s.Require().Nil(segment)
}

func (s *ShardRotatedSuite) TestUnknownFormatVersion() {
	shardID := uint16(0)
	dataDir := s.T().TempDir()
	shardFileName := filepath.Join(dataDir, "shard_0.wal")
	shardFile, err := os.OpenFile( // #nosec G304 // it's meant to be that way
		shardFileName,
		os.O_CREATE|os.O_RDWR,
		0o600, //revive:disable-line:add-constant // file permissions simple readable as octa-number
	)
	s.Require().NoError(err)
	_, err = writer.WriteHeader(shardFile, 42, cppbridge.EncodersVersion())
	s.Require().NoError(err)
	s.Require().NoError(shardFile.Close())

	decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.state", shardID))

	shard, err := newShardRotated(
		s.T().Name(),
		shardID,
		shardFileName,
		decoderStateFileName,
		true,
		labels.EmptyLabels(),
		[]*cppbridge.RelabelConfig{},
		s.segmentSize,
	)
	s.Require().NotNil(shard)
	s.Require().NoError(err)
	s.Require().True(shard.IsCorrupted())
	defer func() { s.Require().NoError(shard.Close()) }()

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(1)

	segmentID, idErr := shard.SegmentID()
	s.Require().Error(idErr)
	s.Require().ErrorIs(idErr, ErrShardIsCorrupted)
	s.Require().Equal(reader.UnknownSegmentID, segmentID)

	segment, err := shard.ReadSegment(0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().Error(err)
	s.Require().ErrorIs(err, ErrShardIsCorrupted)
	s.Require().Nil(segment)
}

func (s *ShardRotatedSuite) TestV1CorruptedSegmentTruncate() {
	shardID := uint16(0)
	dataDir := s.T().TempDir()
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.state", shardID))
	numberOfSegments := uint32(2)

	err := remotewritertest.WriteToShardWalFileV1Multi(s.T().Context(), shardFilePaths, uint64(numberOfSegments))
	s.Require().NoError(err)

	s.Require().NoError(os.Truncate(shardFilePaths[0], 11))

	shard, err := newShardRotated(
		s.T().Name(),
		shardID,
		shardFilePaths[0],
		decoderStateFileName,
		true,
		labels.EmptyLabels(),
		[]*cppbridge.RelabelConfig{},
		s.segmentSize,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(shard.Close()) }()

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(1)

	segmentID, idErr := shard.SegmentID()
	s.Require().NoError(idErr)
	s.Require().Equal(uint32(0), segmentID)

	segment, readErr := shard.ReadSegment(0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().Error(readErr)
	s.Require().ErrorIs(readErr, ErrShardIsCorrupted)
	s.Require().ErrorIs(readErr, io.ErrUnexpectedEOF)
	s.Require().Nil(segment)
}

func (s *ShardRotatedSuite) TestV2CorruptedSegmentTruncate() {
	shardID := uint16(0)
	dataDir := s.T().TempDir()
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.state", shardID))
	numberOfSegments := uint32(2)
	rec := remotewritertest.MakeRecord(1)

	err := remotewritertest.WriteToShardWalFileV2Multi(s.T().Context(), shardFilePaths, uint64(numberOfSegments), rec)
	s.Require().NoError(err)

	s.Require().NoError(os.Truncate(shardFilePaths[0], 11))

	shard, err := newShardRotated(
		s.T().Name(),
		shardID,
		shardFilePaths[0],
		decoderStateFileName,
		true,
		labels.EmptyLabels(),
		[]*cppbridge.RelabelConfig{},
		s.segmentSize,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(shard.Close()) }()

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(1)

	segmentID, idErr := shard.SegmentID()
	s.Require().NoError(idErr)
	s.Require().Equal(uint32(0), segmentID)

	segment, readErr := shard.ReadSegment(0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().Error(readErr)
	s.Require().ErrorIs(readErr, ErrShardIsCorrupted)
	s.Require().ErrorIs(readErr, io.ErrUnexpectedEOF)
	s.Require().Nil(segment)
}

func (s *ShardRotatedSuite) TestV1CorruptedSegmentChangeSegmentByte() {
	shardID := uint16(0)
	dataDir := s.T().TempDir()
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.state", shardID))
	numberOfSegments := uint32(2)

	err := remotewritertest.WriteToShardWalFileV1Multi(s.T().Context(), shardFilePaths, uint64(numberOfSegments))
	s.Require().NoError(err)

	s.Require().NoError(s.corruptSegment(shardFilePaths[0]))

	shard, err := newShardRotated(
		s.T().Name(),
		shardID,
		shardFilePaths[0],
		decoderStateFileName,
		true,
		labels.EmptyLabels(),
		[]*cppbridge.RelabelConfig{},
		s.segmentSize,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(shard.Close()) }()

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(1)

	segmentID, idErr := shard.SegmentID()
	s.Require().NoError(idErr)
	s.Require().Equal(uint32(0), segmentID)

	segment, readErr := shard.ReadSegment(0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().Error(readErr)
	s.Require().ErrorIs(readErr, ErrShardIsCorrupted)
	s.Require().Nil(segment)
}

func (s *ShardRotatedSuite) TestV2CorruptedSegmentChangeSegmentByte() {
	shardID := uint16(0)
	dataDir := s.T().TempDir()
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.state", shardID))
	numberOfSegments := uint32(2)
	rec := remotewritertest.MakeRecord(1)

	err := remotewritertest.WriteToShardWalFileV2Multi(s.T().Context(), shardFilePaths, uint64(numberOfSegments), rec)
	s.Require().NoError(err)

	s.Require().NoError(s.corruptSegment(shardFilePaths[0]))

	shard, err := newShardRotated(
		s.T().Name(),
		shardID,
		shardFilePaths[0],
		decoderStateFileName,
		true,
		labels.EmptyLabels(),
		[]*cppbridge.RelabelConfig{},
		s.segmentSize,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(shard.Close()) }()

	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(1)

	segmentID, idErr := shard.SegmentID()
	s.Require().NoError(idErr)
	s.Require().Equal(uint32(0), segmentID)

	segment, readErr := shard.ReadSegment(0, segmentSampleStorages.Get(uint64(shardID)))
	s.Require().Error(readErr)
	s.Require().ErrorIs(readErr, ErrShardIsCorrupted)
	s.Require().Nil(segment)
}

func (*ShardRotatedSuite) corruptSegment(fileName string) error {
	f, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err = f.Seek(11, 0); err != nil {
		return err
	}

	if _, err = f.Write([]byte{0x42}); err != nil {
		return err
	}

	return nil
}
