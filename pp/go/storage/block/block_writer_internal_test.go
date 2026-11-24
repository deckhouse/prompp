package block

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewBlockWriterCreatesNoFiles(t *testing.T) {
	// Arrange
	dataDir := t.TempDir()

	// Act
	writer := newBlockWriter(dataDir, DefaultChunkSegmentSize, nil, ChunkIterator{}, nil)

	// Assert
	entries, err := os.ReadDir(dataDir)
	require.NoError(t, err)
	require.Empty(t, entries)
	require.True(t, writer.isEmpty())
}

func TestCloseNotOpenedBlockWriter(t *testing.T) {
	// Arrange
	dataDir := t.TempDir()
	writer := newBlockWriter(dataDir, DefaultChunkSegmentSize, nil, ChunkIterator{}, nil)

	// Act
	err := writer.Close()

	// Assert
	require.NoError(t, err)
	entries, err := os.ReadDir(dataDir)
	require.NoError(t, err)
	require.Empty(t, entries)
}
