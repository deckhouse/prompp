package block_test

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/pp-pkg/handler/storage"
	"github.com/prometheus/prometheus/pp-pkg/handler/storage/block"
	"github.com/prometheus/prometheus/util/pool"
)

func TestStorage(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "storage-")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	buffers := pool.New(8, 100e3, 2, func(sz int) interface{} { return make([]byte, 0, sz) })

	blockStorage := block.NewStorage(tempDir, buffers)

	blockID := uuid.New()
	shardID := uint16(1)

	_, err = blockStorage.Reader(blockID, shardID)
	require.ErrorIs(t, err, storage.ErrNoBlock)

	encodedSegment := &model.Segment{
		ID:   0,
		Size: 4,
		CRC:  42,
		Body: make([]byte, 4),
	}

	blockWriter := blockStorage.Writer(blockID, shardID, 0, 1)
	err = blockWriter.Append(encodedSegment)
	require.NoError(t, err)

	err = blockWriter.Close()
	require.NoError(t, err)

	require.Equal(t, uint8(1), blockWriter.Header().FileVersion)
	require.Equal(t, blockID, blockWriter.Header().BlockID)
	require.Equal(t, shardID, blockWriter.Header().ShardID)
	require.Equal(t, uint8(0), blockWriter.Header().ShardLog)
	require.Equal(t, uint8(1), blockWriter.Header().SegmentEncodingVersion)

	blockReader, err := blockStorage.Reader(blockID, shardID)
	require.NoError(t, err)

	rSeg, err := blockReader.Next()
	require.NoError(t, err)

	require.Equal(t, encodedSegment.ID, rSeg.ID)
	require.Equal(t, encodedSegment.Size, rSeg.Size)
	require.Equal(t, encodedSegment.CRC, rSeg.CRC)
	require.EqualValues(t, encodedSegment.Body, rSeg.Body)

	err = blockReader.Close()
	require.NoError(t, err)

	err = blockStorage.Delete(blockID, shardID)
	require.NoError(t, err)
}
