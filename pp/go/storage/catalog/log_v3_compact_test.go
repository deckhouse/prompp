package catalog

import (
	"bytes"
	"io"
	"os"

	"github.com/jonboulle/clockwork"
)

func (s *V3FileTestSuite) TestV3Compact_AlternatesJournalsAndClearsTail() {
	first := newAppendTestRecord(1, 100)
	second := newAppendTestRecord(2, 200)
	s.Require().NoError(s.fl.Write(&first.SerializedRecord))
	s.Require().NoError(s.fl.Write(&second.SerializedRecord))
	s.Require().NoError(os.Mkdir(s.filePath+".compacted", 0o700))
	info, err := os.Stat(s.filePath)
	s.Require().NoError(err)

	s.Require().NoError(s.fl.ReWrite(&first.SerializedRecord, &second.SerializedRecord))
	header, _, err := readFileHeaderV3(s.fl.file.file)
	s.Require().NoError(err)
	s.Equal(uint8(1), header.active)
	s.Require().NoError(s.fl.ReWrite(&second.SerializedRecord))
	s.reopen()

	header, _, err = readFileHeaderV3(s.fl.file.file)
	s.Require().NoError(err)
	s.Equal(uint8(0), header.active)
	s.requireRecords(second)
	after, err := os.Stat(s.filePath)
	s.Require().NoError(err)
	s.True(os.SameFile(info, after), "compaction must overwrite the same allocated file")
	s.requireV3FileSizeOnDisk(s.filePath)
	tail := make([]byte, journalSizeV3-int(header.activeUsed))
	_, err = s.fl.file.file.ReadAt(tail, journalOffsetV3(header.active)+int64(header.activeUsed))
	s.Require().NoError(err)
	s.True(allZeroV3(tail))
}

func (s *V3FileTestSuite) TestV3Compact_CrashBeforeHeaderCommitKeepsOldJournal() {
	first := newAppendTestRecord(1, 100)
	second := newAppendTestRecord(2, 200)
	s.Require().NoError(s.fl.Write(&first.SerializedRecord))
	s.Require().NoError(s.fl.Write(&second.SerializedRecord))
	s.reopen()
	header, slotIndex, err := readFileHeaderV3(s.fl.file.file)
	s.Require().NoError(err)
	frame, err := appendFrameV3(nil, &first.SerializedRecord)
	s.Require().NoError(err)
	_, err = s.fl.file.file.WriteAt(append(frame, 0), journalOffsetV3(header.active^1))
	s.Require().NoError(err)
	s.Require().NoError(s.fl.file.Sync())

	s.crashAndReopen()
	s.requireRecords(first, second)

	// A torn header update also leaves the old slot authoritative.
	header.active ^= 1
	header.generation++
	header.activeUsed = uint32(len(frame))
	slot := encodeFileHeaderV3(header)
	_, err = s.fl.file.file.WriteAt(slot[:fileHeaderChecksumOffsetV3], int64(slotIndex^1)*fileHeaderSlotSizeV3)
	s.Require().NoError(err)
	s.Require().NoError(s.fl.file.Sync())
	s.crashAndReopen()
	s.requireRecords(first, second)
}

func (s *V3FileTestSuite) TestV3Compact_OverflowPreservesSource() {
	record := newAppendTestRecord(1, 100)
	s.Require().NoError(s.fl.Write(&record.SerializedRecord))
	before, err := os.ReadFile(s.filePath)
	s.Require().NoError(err)
	records := make([]*SerializedRecord, 100000)
	for index := range records {
		records[index] = &record.SerializedRecord
	}

	s.Require().ErrorIs(s.fl.ReWrite(records...), ErrJournalFull)

	after, err := os.ReadFile(s.filePath)
	s.Require().NoError(err)
	s.True(bytes.Equal(before, after))
	s.requireRecords(record)
}

func (s *V3FileTestSuite) TestV3Compact_ThresholdSwitchesBeforeMutation() {
	c, err := New(clockwork.NewFakeClock(), s.fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)
	_, err = c.Create(2)
	s.Require().NoError(err)
	s.Require().NoError(os.Mkdir(s.filePath+".compacted", 0o700))
	s.fl.file.writeOffset = s.fl.file.journalEnd - 1

	_, err = c.Create(4)

	s.Require().NoError(err)
	header, _, err := readFileHeaderV3(s.fl.file.file)
	s.Require().NoError(err)
	s.Equal(uint8(1), header.active)
	s.requireV3FileSizeOnDisk(s.filePath)
	s.reopen()
	c, err = New(clockwork.NewFakeClock(), s.fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)
	s.Len(c.List(nil, nil), 2)
}

func (s *V3FileTestSuite) TestV3Read_MissingTerminatorIsCorruption() {
	frame, err := appendFrameV3(nil, &SerializedRecord{})
	s.Require().NoError(err)
	offset := s.fl.file.journalEnd - int64(len(frame))
	_, err = s.fl.file.file.WriteAt(frame, offset)
	s.Require().NoError(err)
	s.fl.file.readOffset = offset

	var got SerializedRecord
	s.Require().NoError(s.fl.Read(&got))
	var damaged *CorruptedRecordError
	s.Require().ErrorAs(s.fl.Read(&got), &damaged)
	s.Require().ErrorIs(damaged, io.ErrUnexpectedEOF)
	s.Require().NotErrorIs(damaged, io.EOF)
	s.Require().ErrorIs(s.fl.Read(&got), io.EOF)
}

func (s *V3FileTestSuite) TestV3Read_ResynchronizesDamagedFrames() {
	for _, damage := range []struct {
		name   string
		offset int64
		mask   byte
	}{
		{name: "checksum", offset: int64(recordChecksumOffsetV3), mask: 0xFF},
		{name: "length", offset: int64(recordLengthOffsetV3), mask: 0xFF},
		{name: "zero version", offset: 0, mask: RecordVersionV3},
		{name: "unknown version", offset: 0, mask: 0xFF},
	} {
		s.Run(damage.name, func() {
			first := newAppendTestRecord(1, 100)
			middle := newAppendTestRecord(2, 200)
			last := newAppendTestRecord(3, 300)
			s.Require().NoError(s.fl.ReWrite(&first.SerializedRecord))
			offset := s.fl.file.writeOffset
			s.Require().NoError(s.fl.Write(&middle.SerializedRecord))
			s.Require().NoError(s.fl.Write(&last.SerializedRecord))
			s.corruptRecordByte(s.fl, offset+damage.offset, damage.mask)
			s.crashAndReopen()

			var got SerializedRecord
			s.Require().NoError(s.fl.Read(&got))
			s.Equal(first.id, got.id)
			var damaged *CorruptedRecordError
			s.Require().ErrorAs(s.fl.Read(&got), &damaged)
			s.Equal(offset, damaged.Offset)
			s.Equal(middle.id, damaged.ID)
			s.Require().NotErrorIs(damaged, io.EOF)
			s.Require().NoError(s.fl.Read(&got))
			s.Equal(last.id, got.id)
			s.Require().ErrorIs(s.fl.Read(&got), io.EOF)
		})
	}
}
