package catalog

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
)

func TestV3Append_TerminatorAfterEachWrite(t *testing.T) {
	// Arrange
	filePath := filepath.Join(t.TempDir(), "head.log")
	fl, err := NewFileLogV3(filePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, fl.Close()) })

	// Act & Assert
	for i := range 3 {
		require.NoError(t, fl.Write(&newAppendTestRecord(uint16(i+1), int64(100+i)).SerializedRecord))

		terminator := make([]byte, 1)
		_, err = readFileBytesAt(t, filePath, terminator, fl.file.writeOffset)
		require.NoError(t, err)
		require.Equal(t, byte(recordVersionEndV3), terminator[0], "write %d", i)

		require.Equal(t, fileSizeV3, fl.Size())

		info, err := os.Stat(filePath)
		require.NoError(t, err)
		require.Equal(t, int64(fileSizeV3), info.Size(), "write %d", i)
	}
}

func TestV3Append_ReopenReadsBackRecordsInOrder(t *testing.T) {
	// Arrange
	filePath := filepath.Join(t.TempDir(), "head.log")
	fl, err := NewFileLogV3(filePath)
	require.NoError(t, err)

	expected := []*Record{
		newAppendTestRecord(1, 100),
		newAppendTestRecord(2, 200),
		newAppendTestRecord(3, 300),
	}
	for _, record := range expected {
		require.NoError(t, fl.Write(&record.SerializedRecord))
	}
	require.NoError(t, fl.Close())

	// Act
	reopened, err := NewFileLogV3(filePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })

	// Assert
	for i, want := range expected {
		var got SerializedRecord
		require.NoError(t, reopened.Read(&got), "record %d", i)
		require.Equal(t, want.SerializedRecord, got, "record %d", i)
	}

	var got SerializedRecord
	require.ErrorIs(t, reopened.Read(&got), io.EOF)

	info, err := os.Stat(filePath)
	require.NoError(t, err)
	require.Equal(t, int64(fileSizeV3), info.Size())
}

func TestV3Append_StaleDataAfterTerminatorIsNotRead(t *testing.T) {
	// Arrange
	filePath := filepath.Join(t.TempDir(), "head.log")
	fl, err := NewFileLogV3(filePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, fl.Close()) })

	record := newAppendTestRecord(1, 100)
	require.NoError(t, fl.Write(&record.SerializedRecord))

	stale := newAppendTestRecord(2, 200)
	staleFrame, err := appendFrameV3(nil, &stale.SerializedRecord)
	require.NoError(t, err)

	// Act: craft [rec1][0x00][stale rec2][garbage] keeping the terminator in place.
	_, err = fl.file.file.WriteAt(append(staleFrame, 0xAA, 0xBB), fl.file.writeOffset+1)
	require.NoError(t, err)
	require.NoError(t, fl.file.Sync())

	// Assert: the sequential read stops at the terminator and never reaches the stale data.
	var got SerializedRecord
	require.NoError(t, fl.Read(&got))
	require.Equal(t, record.SerializedRecord, got)
	require.ErrorIs(t, fl.Read(&got), io.EOF)

	require.Equal(t, fileSizeV3, fl.Size())

	info, err := os.Stat(filePath)
	require.NoError(t, err)
	require.Equal(t, int64(fileSizeV3), info.Size())
}

func TestV3Append_TornTailIsDroppedOnOpen(t *testing.T) {
	// Arrange
	filePath := filepath.Join(t.TempDir(), "head.log")
	fl, err := NewFileLogV3(filePath)
	require.NoError(t, err)

	kept := newAppendTestRecord(1, 100)
	require.NoError(t, fl.Write(&kept.SerializedRecord))

	tornFrameOffset := fl.file.writeOffset
	torn := newAppendTestRecord(2, 200)
	require.NoError(t, fl.Write(&torn.SerializedRecord))

	// Act: zero the second half of the last frame in place, keeping the file size.
	frameLen := int(fl.file.writeOffset - tornFrameOffset)
	_, err = fl.file.file.WriteAt(make([]byte, frameLen-frameLen/2), tornFrameOffset+int64(frameLen/2))
	require.NoError(t, err)
	require.NoError(t, fl.file.Sync())

	// Assert: the torn frame fails the checksum instead of looking like a clean end.
	var keptGot SerializedRecord
	require.NoError(t, fl.Read(&keptGot))
	require.Equal(t, kept.SerializedRecord, keptGot)

	var got SerializedRecord
	err = fl.Read(&got)
	require.ErrorIs(t, err, ErrRecordChecksumMismatch)
	require.NotErrorIs(t, err, io.EOF)

	require.NoError(t, fl.Close())

	info, err := os.Stat(filePath)
	require.NoError(t, err)
	require.Equal(t, int64(fileSizeV3), info.Size())

	// Act: the catalog loads the prior records and drops the torn tail on the rewrite.
	l, err := NewFileLogV3(filePath)
	require.NoError(t, err)

	c, err := New(clockwork.NewFakeClock(), l, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, l.Close()) })

	// Assert
	records := c.List(nil, nil)
	require.Len(t, records, 1)
	require.Equal(t, kept.ID(), records[0].ID())
}

func TestV3Close_PersistsActiveUsedAndTerminator(t *testing.T) {
	// Arrange
	filePath := filepath.Join(t.TempDir(), "head.log")
	fl, err := NewFileLogV3(filePath)
	require.NoError(t, err)

	expected := []*Record{
		newAppendTestRecord(1, 100),
		newAppendTestRecord(2, 200),
	}
	for _, record := range expected {
		require.NoError(t, fl.Write(&record.SerializedRecord))
	}

	used := fl.file.writeOffset - journalOffsetV3(0)

	// Act
	require.NoError(t, fl.Close())

	// Assert
	reopened, err := NewFileLogV3(filePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })

	header, err := readFileHeaderV3(reopened.file.file)
	require.NoError(t, err)
	require.Zero(t, header.active)
	require.Equal(t, uint64(2), header.generation)
	require.Equal(t, uint32(used), header.activeUsed)

	require.Equal(t, journalOffsetV3(0), reopened.file.readOffset)
	require.Equal(t, journalOffsetV3(0)+used, reopened.file.writeOffset)

	terminator := make([]byte, 1)
	_, err = readFileBytesAt(t, filePath, terminator, journalOffsetV3(0)+used)
	require.NoError(t, err)
	require.Equal(t, byte(recordVersionEndV3), terminator[0])

	for i, want := range expected {
		var got SerializedRecord
		require.NoError(t, reopened.Read(&got), "record %d", i)
		require.Equal(t, want.SerializedRecord, got, "record %d", i)
	}

	var got SerializedRecord
	require.ErrorIs(t, reopened.Read(&got), io.EOF)
}

func TestV3Close_HeaderRewriteErrorStillClosesFile(t *testing.T) {
	// Arrange
	filePath := filepath.Join(t.TempDir(), "head.log")
	fl, err := NewFileLogV3(filePath)
	require.NoError(t, err)
	require.NoError(t, fl.Write(&newAppendTestRecord(1, 100).SerializedRecord))

	corruptV3HeaderSlot(t, filePath, fileHeaderGenerationOffsetV3)
	corruptV3HeaderSlot(t, filePath, fileHeaderSlotSizeV3+fileHeaderGenerationOffsetV3)

	// Act
	err = fl.Close()

	// Assert
	require.ErrorIs(t, err, ErrInvalidV3Header)
	require.ErrorIs(t, fl.file.file.Close(), os.ErrClosed)
}

func newAppendTestRecord(numberOfShards uint16, createdAt int64) *Record {
	return NewRecordWithDataV3(uuid.New(), numberOfShards, createdAt, createdAt, 0, false, StatusNew, 3, 10, 20)
}
