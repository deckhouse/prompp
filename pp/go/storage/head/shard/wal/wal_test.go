package wal_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer"
)

// TODO moq -out wal_moq_test.go -pkg wal_test -rm . ReadSegment EncodedSegment SegmentWriter Encoder StatsSegment

func TestXxx(t *testing.T) {
	shardID := uint16(0)
	tmpDir, err := os.MkdirTemp("", "shard")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	shardFile, err := os.Create(filepath.Join(filepath.Clean(tmpDir), fmt.Sprintf("shard_%d.wal", shardID)))
	require.NoError(t, err)

	swn := &segmentWriteNotifier{}

	defer func() {
		if err == nil {
			return
		}
		_ = shardFile.Close()
	}()

	sw, err := writer.NewBuffered(shardID, shardFile, writer.WriteSegment[*cppbridge.EncodedSegment], swn)
	require.NoError(t, err)

	shardWalEncoder := &cppbridge.HeadWalEncoder{}

	wl := wal.NewWal(shardWalEncoder, sw, 10)
	_ = wl
}

// segmentWriteNotifier test implementation [writer.SegmentIsWrittenNotifier].
type segmentWriteNotifier struct{}

// NotifySegmentIsWritten test implementation [writer.SegmentIsWrittenNotifier].
func (*segmentWriteNotifier) NotifySegmentIsWritten(shardID uint16) {
	_ = shardID
}

type WalSuite struct {
	suite.Suite
}

func TestWalSuite(t *testing.T) {
	suite.Run(t, new(WalSuite))
}

func (s *WalSuite) TestCurrentSize() {
	expectedWalSize := int64(42)
	enc := &EncoderMock[*EncodedSegmentMock, *StatsSegmentMock]{}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		CurrentSizeFunc: func() int64 {
			return expectedWalSize
		},
	}
	maxSegmentSize := uint32(100)

	wl := wal.NewWal(enc, segmentWriter, maxSegmentSize)

	s.Equal(expectedWalSize, wl.CurrentSize())
}

func (s *WalSuite) TestCurrentSize2() {
	maxSegmentSize := uint32(100)
	// enSegment := &EncodedSegmentMock{}
	// stats := &StatsSegmentMock{}
	enc := &EncoderMock[*EncodedSegmentMock, *StatsSegmentMock]{}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{}

	wl := wal.NewWal(enc, segmentWriter, maxSegmentSize)
	_ = wl
}
