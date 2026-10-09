package catalog

import (
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestV3WritesNeedNoDiskSpace(t *testing.T) {
	// Arrange: creation preallocates the fixed size — the only operation that needs disk space.
	filePath := filepath.Join(t.TempDir(), "head.log")
	fl, err := NewFileLogV3(filePath)
	require.NoError(t, err)

	requireV3FileSizeOnDisk(t, filePath)

	// Act: arm growth refusal; appends must keep succeeding without growing the file.
	fl.file.refuseGrowth = true

	expected := make([]*Record, 5)
	for i := range expected {
		expected[i] = newAppendTestRecord(uint16(i+1), int64(100+i))
		require.NoError(t, fl.Write(&expected[i].SerializedRecord), "append %d", i)
		require.Equal(t, fileSizeV3, fl.Size(), "append %d", i)
		requireV3FileSizeOnDisk(t, filePath)
	}

	// Act: the Close activeUsed header rewrite must also succeed under refusal.
	require.NoError(t, fl.Close())
	requireV3FileSizeOnDisk(t, filePath)

	reopened, err := NewFileLogV3(filePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })

	reopened.file.refuseGrowth = true

	// Assert: every record reads back in order, then a clean end, size unchanged throughout.
	for i, want := range expected {
		var got SerializedRecord
		require.NoError(t, reopened.Read(&got), "record %d", i)
		require.Equal(t, want.SerializedRecord, got, "record %d", i)
	}

	var got SerializedRecord
	require.ErrorIs(t, reopened.Read(&got), io.EOF)
	requireV3FileSizeOnDisk(t, filePath)

	// Negative control: a positioned write past the accounted size is refused with ENOSPC.
	_, err = reopened.file.writeAt([]byte{0xAA}, int64(fileSizeV3))
	require.ErrorIs(t, err, syscall.ENOSPC)

	// Negative control through the production append path: WriteFixed at the end refuses too.
	writeOffset := reopened.file.writeOffset
	reopened.file.SetWriteOffset(int64(fileSizeV3))
	require.ErrorIs(t, reopened.file.WriteFixed([]byte{0xAA}), syscall.ENOSPC)
	reopened.file.SetWriteOffset(writeOffset)

	// A write ending exactly at fileSizeV3 stays an overwrite and goes through.
	n, err := reopened.file.writeAt([]byte{0}, int64(fileSizeV3)-1)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	requireV3FileSizeOnDisk(t, filePath)
}

// requireV3FileSizeOnDisk asserts that the log file version 3 keeps its fixed size on disk.
func requireV3FileSizeOnDisk(t *testing.T, filePath string) {
	t.Helper()

	info, err := os.Stat(filePath)
	require.NoError(t, err)
	require.Equal(t, int64(fileSizeV3), info.Size())
}
