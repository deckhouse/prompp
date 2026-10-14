package catalog

import (
	"io"
	"os"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
)

func (s *V3FileTestSuite) TestV3Append_ReopenReadsBackRecordsInOrder() {
	// Arrange
	filePath := s.filePath
	fl := s.fl
	var err error

	expected := []*Record{
		newAppendTestRecord(1, 100),
		newAppendTestRecord(2, 200),
		newAppendTestRecord(3, 300),
	}
	for _, record := range expected {
		s.Require().NoError(fl.Write(&record.SerializedRecord))
	}
	s.Require().NoError(fl.Close())

	// Act
	reopened, err := NewFileLogV3(filePath)
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(reopened.Close()) })

	// Assert
	for i, want := range expected {
		var got SerializedRecord
		s.Require().NoError(reopened.Read(&got), "record %d", i)
		s.Require().Equal(want.SerializedRecord, got, "record %d", i)
	}

	var got SerializedRecord
	s.Require().ErrorIs(reopened.Read(&got), io.EOF)

	info, err := os.Stat(filePath)
	s.Require().NoError(err)
	s.Require().Equal(int64(fileSizeV3), info.Size())
}

func (s *V3FileTestSuite) TestV3Append_StaleDataAfterTerminatorIsNotRead() {
	// Arrange
	filePath := s.filePath
	fl := s.fl
	var err error

	record := newAppendTestRecord(1, 100)
	s.Require().NoError(fl.Write(&record.SerializedRecord))

	stale := newAppendTestRecord(2, 200)
	staleFrame, err := appendFrameV3(nil, &stale.SerializedRecord)
	s.Require().NoError(err)

	// Act: craft [rec1][0x00][stale rec2][garbage] keeping the terminator in place.
	_, err = fl.file.file.WriteAt(append(staleFrame, 0xAA, 0xBB), fl.file.writeOffset+1)
	s.Require().NoError(err)
	s.Require().NoError(fl.file.Sync())

	// Assert: the sequential read stops at the terminator and never reaches the stale data.
	var got SerializedRecord
	s.Require().NoError(fl.Read(&got))
	s.Require().Equal(record.SerializedRecord, got)
	s.Require().ErrorIs(fl.Read(&got), io.EOF)

	s.Require().Equal(fileSizeV3, fl.Size())

	info, err := os.Stat(filePath)
	s.Require().NoError(err)
	s.Require().Equal(int64(fileSizeV3), info.Size())
}

func (s *V3FileTestSuite) TestV3Append_TerminatorAfterEachWrite() {
	// Arrange
	filePath := s.filePath
	fl := s.fl
	var err error

	// Act & Assert
	for i := range 3 {
		s.Require().NoError(fl.Write(&newAppendTestRecord(uint16(i+1), int64(100+i)).SerializedRecord))

		terminator := make([]byte, 1)
		_, err = s.readFileBytesAt(filePath, terminator, fl.file.writeOffset)
		s.Require().NoError(err)
		s.Require().Equal(recordVersionEndV3, terminator[0], "write %d", i)

		s.Require().Equal(fileSizeV3, fl.Size())

		info, err := os.Stat(filePath)
		s.Require().NoError(err)
		s.Require().Equal(int64(fileSizeV3), info.Size(), "write %d", i)
	}
}

func (s *V3FileTestSuite) TestV3Append_TornTailIsDroppedOnOpen() {
	// Arrange
	filePath := s.filePath
	fl := s.fl
	var err error

	kept := newAppendTestRecord(1, 100)
	s.Require().NoError(fl.Write(&kept.SerializedRecord))

	tornFrameOffset := fl.file.writeOffset
	torn := newAppendTestRecord(2, 200)
	s.Require().NoError(fl.Write(&torn.SerializedRecord))

	// Act: zero the second half of the last frame in place, keeping the file size.
	frameLen := int(fl.file.writeOffset - tornFrameOffset)
	_, err = fl.file.file.WriteAt(make([]byte, frameLen-frameLen/2), tornFrameOffset+int64(frameLen/2))
	s.Require().NoError(err)
	s.Require().NoError(fl.file.Sync())

	// Assert: the torn frame fails the checksum instead of looking like a clean end.
	var keptGot SerializedRecord
	s.Require().NoError(fl.Read(&keptGot))
	s.Require().Equal(kept.SerializedRecord, keptGot)

	var got SerializedRecord
	err = fl.Read(&got)
	s.Require().ErrorIs(err, ErrRecordChecksumMismatch)
	s.Require().NotErrorIs(err, io.EOF)

	s.Require().NoError(fl.Close())

	info, err := os.Stat(filePath)
	s.Require().NoError(err)
	s.Require().Equal(int64(fileSizeV3), info.Size())

	// Act: the catalog loads the prior records and drops the torn tail on the rewrite.
	l, err := NewFileLogV3(filePath)
	s.Require().NoError(err)

	c, err := New(clockwork.NewFakeClock(), l, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(l.Close()) })

	// Assert
	records := c.List(nil, nil)
	s.Require().Len(records, 1)
	s.Require().Equal(kept.ID(), records[0].ID())
}

func (s *V3FileTestSuite) TestV3Close_HeaderRewriteErrorStillClosesFile() {
	// Arrange
	filePath := s.filePath
	fl := s.fl
	var err error
	s.Require().NoError(fl.Write(&newAppendTestRecord(1, 100).SerializedRecord))

	s.corruptV3HeaderSlot(filePath, fileHeaderGenerationOffsetV3)
	s.corruptV3HeaderSlot(filePath, fileHeaderSlotSizeV3+fileHeaderGenerationOffsetV3)

	// Act
	err = fl.Close()

	// Assert
	s.Require().ErrorIs(err, ErrInvalidV3Header)
	s.Require().ErrorIs(fl.file.file.Close(), os.ErrClosed)
}

func (s *V3FileTestSuite) TestV3Close_PersistsActiveUsedAndTerminator() {
	// Arrange
	filePath := s.filePath
	fl := s.fl
	var err error

	expected := []*Record{
		newAppendTestRecord(1, 100),
		newAppendTestRecord(2, 200),
	}
	for _, record := range expected {
		s.Require().NoError(fl.Write(&record.SerializedRecord))
	}

	used := fl.file.writeOffset - journalOffsetV3(0)

	// Act
	s.Require().NoError(fl.Close())

	// Assert
	reopened, err := NewFileLogV3(filePath)
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(reopened.Close()) })

	header, _, err := readFileHeaderV3(reopened.file.file)
	s.Require().NoError(err)
	s.Require().Zero(header.active)
	s.Require().Equal(uint64(2), header.generation)
	s.Require().Equal(uint32(used), header.activeUsed)

	s.Require().Equal(journalOffsetV3(0), reopened.file.readOffset)
	s.Require().Equal(journalOffsetV3(0)+used, reopened.file.writeOffset)

	terminator := make([]byte, 1)
	_, err = s.readFileBytesAt(filePath, terminator, journalOffsetV3(0)+used)
	s.Require().NoError(err)
	s.Require().Equal(recordVersionEndV3, terminator[0])

	for i, want := range expected {
		var got SerializedRecord
		s.Require().NoError(reopened.Read(&got), "record %d", i)
		s.Require().Equal(want.SerializedRecord, got, "record %d", i)
	}

	var got SerializedRecord
	s.Require().ErrorIs(reopened.Read(&got), io.EOF)
}

func newAppendTestRecord(numberOfShards uint16, createdAt int64) *Record {
	return NewRecordWithDataV3(uuid.New(), numberOfShards, createdAt, createdAt, 0, false, StatusNew, 3, 10, 20)
}
