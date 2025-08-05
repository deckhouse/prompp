package wal_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/writer"
	"github.com/stretchr/testify/require"
)

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

	sw, err := writer.NewBuffered(shardID, shardFile, writer.WriteSegment[wal.EncodedSegment], swn)
	require.NoError(t, err)

	shardWalEncoder := &cppbridge.HeadWalEncoder{}

	wal.NewWal(shardWalEncoder, 10, sw)
}

// segmentWriteNotifier test implementation [writer.SegmentIsWrittenNotifier].
type segmentWriteNotifier struct{}

// NotifySegmentIsWritten test implementation [writer.SegmentIsWrittenNotifier].
func (*segmentWriteNotifier) NotifySegmentIsWritten(shardID uint16) {
	_ = shardID
}
