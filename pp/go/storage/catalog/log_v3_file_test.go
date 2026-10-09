package catalog

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateV3File_PreallocatedSize(t *testing.T) {
	// Arrange
	filePath := filepath.Join(t.TempDir(), "head.log")

	// Act
	fl, err := NewFileLogV3(filePath)

	// Assert
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, fl.Close()) })

	require.Equal(t, fileSizeV3, fl.Size())

	info, err := os.Stat(filePath)
	require.NoError(t, err)
	require.Equal(t, int64(fileSizeV3), info.Size())

	firstByte := make([]byte, 1)
	_, err = readFileBytesAt(t, filePath, firstByte, 0)
	require.NoError(t, err)
	require.Equal(t, byte(LogFileVersionV3), firstByte[0])
}

func TestReopenV3File_EmptyJournal(t *testing.T) {
	// Arrange
	filePath := filepath.Join(t.TempDir(), "head.log")
	fl, err := NewFileLogV3(filePath)
	require.NoError(t, err)
	require.NoError(t, fl.Close())

	// Act
	reopened, err := NewFileLogV3(filePath)

	// Assert
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })

	require.Equal(t, fileSizeV3, reopened.Size())

	info, err := os.Stat(filePath)
	require.NoError(t, err)
	require.Equal(t, int64(fileSizeV3), info.Size())

	header, err := readFileHeaderV3(reopened.file.file)
	require.NoError(t, err)
	require.Zero(t, header.active)
	require.Equal(t, uint64(1), header.generation)
	require.Zero(t, header.activeUsed)

	require.Equal(t, journalOffsetV3(0), reopened.file.readOffset)
	require.Equal(t, journalOffsetV3(0), reopened.file.writeOffset)

	emptyJournal := make([]byte, journalSizeV3)
	for index := uint8(0); index < fileHeaderSlotsCount; index++ {
		journal := make([]byte, journalSizeV3)
		_, err = reopened.file.file.ReadAt(journal, journalOffsetV3(index))
		require.NoError(t, err)
		require.True(t, bytes.Equal(emptyJournal, journal), "journal %d is not empty", index)
	}

	sr := NewEmptyRecord()
	require.ErrorIs(t, reopened.Read(&sr.SerializedRecord), io.EOF)
	require.Equal(t, fileSizeV3, reopened.Size())
}

func TestV3Header_CorruptedSlot(t *testing.T) {
	// Arrange
	filePath := filepath.Join(t.TempDir(), "head.log")
	fl, err := NewFileLogV3(filePath)
	require.NoError(t, err)
	require.NoError(t, fl.Close())

	// Act: corrupt the generation field of slot 0, keeping the first byte intact.
	corruptV3HeaderSlot(t, filePath, fileHeaderGenerationOffsetV3)

	// Assert: the valid slot 1 wins.
	reopened, err := NewFileLogV3(filePath)
	require.NoError(t, err)

	header, err := readFileHeaderV3(reopened.file.file)
	require.NoError(t, err)
	require.Zero(t, header.active)
	require.Equal(t, uint64(1), header.generation)
	require.NoError(t, reopened.Close())

	// Act: corrupt the same field of slot 1, so both slots fail the CRC check.
	corruptV3HeaderSlot(t, filePath, fileHeaderSlotSizeV3+fileHeaderGenerationOffsetV3)

	// Assert: opening fails with an explicit error.
	_, err = NewFileLogV3(filePath)
	require.ErrorIs(t, err, ErrInvalidV3Header)
}

// corruptV3HeaderSlot overwrites one byte of the header slot at the given offset inside the slot.
func corruptV3HeaderSlot(t *testing.T, filePath string, offset int) {
	t.Helper()

	f, err := os.OpenFile(filePath, os.O_RDWR, logFilePerm)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()

	_, err = f.WriteAt([]byte{0xFF}, int64(offset))
	require.NoError(t, err)
	require.NoError(t, f.Sync())
}

func readFileBytesAt(t *testing.T, filePath string, p []byte, offset int64) (int, error) {
	t.Helper()

	f, err := os.Open(filePath) //nolint:gosec // test reads the file it created
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()

	return f.ReadAt(p, offset)
}
