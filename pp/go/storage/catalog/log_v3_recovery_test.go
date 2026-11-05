package catalog

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"os"

	"github.com/jonboulle/clockwork"
	"google.golang.org/protobuf/encoding/protowire"
)

func (s *V3FileTestSuite) TestV3Append_CrashAfterCheckpoint() {
	first := newAppendTestRecord(1, 100)
	second := newAppendTestRecord(2, 200)
	third := newAppendTestRecord(3, 300)
	s.Require().NoError(s.fl.Write(&first.SerializedRecord))
	s.reopen()
	s.Require().NoError(s.fl.Write(&second.SerializedRecord))

	s.crashAndReopen()
	s.Require().NoError(s.fl.Write(&third.SerializedRecord))
	s.reopen()

	s.requireRecords(first, second, third)
}

func (s *V3FileTestSuite) TestV3Append_CrashRecoveryThroughCatalog() {
	first := newAppendTestRecord(1, 100)
	s.Require().NoError(s.fl.Write(&first.SerializedRecord))
	s.crashAndReopen()
	c, err := New(clockwork.NewFakeClock(), s.fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)
	s.Require().Len(c.List(nil, nil), 1)

	_, err = c.Create(2)
	s.Require().NoError(err)
	s.Require().True(s.fl.isFixedV3())
	s.requireV3FileSizeOnDisk(s.filePath)
	s.reopen()

	c, err = New(clockwork.NewFakeClock(), s.fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)
	s.Require().Len(c.List(nil, nil), 2)
}

func (s *V3FileTestSuite) TestV3Append_FullJournalSurvivesReopen() {
	frame, err := appendFrameV3(nil, &SerializedRecord{})
	s.Require().NoError(err)
	data := bytes.Repeat(frame, (journalSizeV3-1)/len(frame)-2)
	remaining := journalSizeV3 - 1 - len(data)
	tail := make([]byte, recordHeaderSizeV3)
	tail[0] = RecordVersionV3
	tail = protowire.AppendTag(tail, 50, protowire.BytesType)
	tail = protowire.AppendBytes(tail, make([]byte, remaining-recordHeaderSizeV3-3))
	tail[recordLengthOffsetV3] = byte(len(tail) - recordHeaderSizeV3)
	binary.LittleEndian.PutUint32(tail[recordChecksumOffsetV3:], checksumV3(tail))
	data = append(data, tail...)
	s.Require().Len(data, journalSizeV3-1)
	data = append(data, recordVersionEndV3)
	_, err = s.fl.file.file.WriteAt(data, s.fl.file.journalStart)
	s.Require().NoError(err)
	s.Require().NoError(s.fl.file.Sync())

	s.crashAndReopen()

	s.Require().Nil(s.fl.file.appendErr)
	s.Require().Equal(s.fl.file.journalEnd-1, s.fl.file.writeOffset)
	s.Require().ErrorIs(s.fl.Write(&newAppendTestRecord(1, 100).SerializedRecord), ErrJournalFull)
	s.requireV3FileSizeOnDisk(s.filePath)
}

func (s *V3FileTestSuite) TestV3Append_JournalOneBoundary() {
	s.requireJournalBoundary(1)
}

func (s *V3FileTestSuite) TestV3Append_JournalZeroBoundary() {
	s.requireJournalBoundary(0)
}

func (s *V3FileTestSuite) TestV3Append_TornTailRequiresRewrite() {
	first := newAppendTestRecord(1, 100)
	second := newAppendTestRecord(2, 200)
	s.Require().NoError(s.fl.Write(&first.SerializedRecord))
	offset := s.fl.file.writeOffset
	s.Require().NoError(s.fl.Write(&second.SerializedRecord))
	_, err := s.fl.file.file.WriteAt([]byte{0xFF}, offset+int64(recordChecksumOffsetV3))
	s.Require().NoError(err)
	s.crashAndReopen()
	before, err := s.fl.file.file.Stat()
	s.Require().NoError(err)

	err = s.fl.Write(&newAppendTestRecord(3, 300).SerializedRecord)

	s.Require().ErrorIs(err, ErrRecordChecksumMismatch)
	s.Require().Equal(before.Size(), int64(s.fl.Size()))
	c, err := New(clockwork.NewFakeClock(), s.fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)
	s.Require().Len(c.List(nil, nil), 1)
	_, err = c.Create(3)
	s.Require().NoError(err)
}

func (s *V3FileTestSuite) TestV3Catalog_FirstMutationKeepsPreallocation() {
	c, err := New(clockwork.NewFakeClock(), s.fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)

	_, err = c.Create(1)

	s.Require().NoError(err)
	s.Require().True(s.fl.isFixedV3())
	s.requireV3FileSizeOnDisk(s.filePath)
}

func (s *V3FileTestSuite) TestV3Close_AlternatesHeaderSlots() {
	s.Require().NoError(s.fl.Write(&newAppendTestRecord(1, 100).SerializedRecord))
	s.reopen()
	header, slot, err := readFileHeaderV3(s.fl.file.file)
	s.Require().NoError(err)
	s.Require().Equal(uint8(1), slot)
	s.Require().Equal(uint64(2), header.generation)
	s.Require().NoError(s.fl.Write(&newAppendTestRecord(2, 200).SerializedRecord))

	s.reopen()

	header, slot, err = readFileHeaderV3(s.fl.file.file)
	s.Require().NoError(err)
	s.Require().Equal(uint8(0), slot)
	s.Require().Equal(uint64(3), header.generation)
	var oldSlot [fileHeaderSlotSizeV3]byte
	_, err = s.fl.file.file.ReadAt(oldSlot[:], fileHeaderSlotSizeV3)
	s.Require().NoError(err)
	previous, err := decodeFileHeaderV3(oldSlot[:])
	s.Require().NoError(err)
	s.Require().Equal(uint64(2), previous.generation)
}

func (s *V3FileTestSuite) TestV3Header_BothSlotsCorruptedPreservesFile() {
	s.Require().NoError(s.fl.Write(&newAppendTestRecord(1, 100).SerializedRecord))
	s.Require().NoError(s.fl.Close())
	s.corruptV3HeaderSlot(s.filePath, fileHeaderVersionOffsetV3)
	f, err := os.OpenFile(s.filePath, os.O_RDWR, logFilePerm)
	s.Require().NoError(err)
	var checksumByte [1]byte
	offset := int64(fileHeaderSlotSizeV3 + fileHeaderChecksumOffsetV3)
	_, err = f.ReadAt(checksumByte[:], offset)
	s.Require().NoError(err)
	checksumByte[0] ^= 1
	_, err = f.WriteAt(checksumByte[:], offset)
	s.Require().NoError(err)
	s.Require().NoError(f.Close())
	before, err := os.ReadFile(s.filePath)
	s.Require().NoError(err)

	fl, err := NewFileLogV3(s.filePath)

	s.Require().ErrorIs(err, ErrInvalidV3Header)
	s.Require().Nil(fl)
	after, err := os.ReadFile(s.filePath)
	s.Require().NoError(err)
	s.Require().True(bytes.Equal(before, after), "opening must not overwrite an invalid fixed-v3 file")
}

func (s *V3FileTestSuite) TestV3Header_FirstVersionByteCorrupted() {
	record := newAppendTestRecord(1, 100)
	s.Require().NoError(s.fl.Write(&record.SerializedRecord))
	s.Require().NoError(s.fl.Close())
	s.corruptV3HeaderSlot(s.filePath, fileHeaderVersionOffsetV3)

	var err error
	s.fl, err = NewFileLogV3(s.filePath)
	s.Require().NoError(err)

	s.requireRecords(record)
}

func (s *V3FileTestSuite) TestV3Header_LegacyVersionByteStillRecoversSecondSlot() {
	record := newAppendTestRecord(1, 100)
	s.Require().NoError(s.fl.Write(&record.SerializedRecord))
	s.Require().NoError(s.fl.Close())
	f, err := os.OpenFile(s.filePath, os.O_RDWR, logFilePerm)
	s.Require().NoError(err)
	_, err = f.WriteAt([]byte{byte(LogFileVersionV2)}, fileHeaderVersionOffsetV3)
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	s.fl, err = NewFileLogV3(s.filePath)
	s.Require().NoError(err)

	_, slot, err := readFileHeaderV3(s.fl.file.file)
	s.Require().NoError(err)
	s.Require().Equal(uint8(1), slot)
	s.requireRecords(record)
}

func (s *V3FileTestSuite) TestV3Header_RejectsUsedWithoutTerminatorSpace() {
	header := fileHeaderV3{generation: 1, journalSize: journalSizeV3, activeUsed: journalSizeV3}
	slot := encodeFileHeaderV3(header)

	_, err := decodeFileHeaderV3(slot[:])

	s.Require().ErrorIs(err, ErrInvalidV3Header)
}

func (s *V3FileTestSuite) TestV3Header_TornLatestSlotKeepsRecords() {
	first := newAppendTestRecord(1, 100)
	second := newAppendTestRecord(2, 200)
	third := newAppendTestRecord(3, 300)
	s.Require().NoError(s.fl.Write(&first.SerializedRecord))
	s.reopen()
	s.Require().NoError(s.fl.Write(&second.SerializedRecord))
	s.Require().NoError(s.fl.Close())
	s.corruptV3HeaderSlot(s.filePath, fileHeaderGenerationOffsetV3)
	var err error
	s.fl, err = NewFileLogV3(s.filePath)
	s.Require().NoError(err)

	s.Require().NoError(s.fl.Write(&third.SerializedRecord))
	s.reopen()

	s.requireRecords(first, second, third)
}

func (s *V3FileTestSuite) TestV3Header_UnexpectedFileSizeIsRejected() {
	s.Require().NoError(s.fl.file.file.Truncate(fileSizeV3 + 1))
	s.Require().NoError(s.fl.file.Close())

	fl, err := NewFileLogV3(s.filePath)

	s.Require().ErrorContains(err, "preallocated file size")
	s.Require().Nil(fl)
}

func (s *V3FileTestSuite) TestV3Open_TerminatorBeforeCheckpointIsAnError() {
	s.Require().NoError(s.fl.Write(&newAppendTestRecord(1, 100).SerializedRecord))
	s.Require().NoError(s.fl.Close())
	f, err := os.OpenFile(s.filePath, os.O_RDWR, logFilePerm)
	s.Require().NoError(err)
	_, err = f.WriteAt([]byte{recordVersionEndV3}, journalOffsetV3(0))
	s.Require().NoError(err)
	s.Require().NoError(f.Close())
	before, err := os.ReadFile(s.filePath)
	s.Require().NoError(err)

	fl, err := NewFileLogV3(s.filePath)

	s.Require().ErrorIs(err, io.ErrUnexpectedEOF)
	s.Require().NotErrorIs(err, io.EOF)
	s.Require().Nil(fl)
	after, err := os.ReadFile(s.filePath)
	s.Require().NoError(err)
	s.Require().True(bytes.Equal(before, after), "opening must preserve the journal with a premature terminator")
}

func (s *V3FileTestSuite) TestV3Read_StopsAtJournalBoundary() {
	s.fl.file.readOffset = s.fl.file.journalEnd

	var record SerializedRecord
	err := s.fl.Read(&record)

	s.Require().ErrorIs(err, io.EOF)
}

func (s *V3FileTestSuite) TestV3WriteFixed_RejectsInvalidOffsets() {
	writeOffset := s.fl.file.writeOffset
	defer func() { s.fl.file.writeOffset = writeOffset }()
	for _, offset := range []int64{-1, fileSizeV3, math.MaxInt64} {
		s.fl.file.writeOffset = offset
		s.Require().Error(s.fl.file.WriteFixed([]byte{0xAA}))
		s.Require().Equal(offset, s.fl.file.writeOffset)
	}

	s.requireV3FileSizeOnDisk(s.filePath)
}

func (s *V3FileTestSuite) crashAndReopen() {
	s.Require().NoError(s.fl.file.Close())
	var err error
	s.fl, err = NewFileLogV3(s.filePath)
	s.Require().NoError(err)
}

func (s *V3FileTestSuite) reopen() {
	s.Require().NoError(s.fl.Close())
	var err error
	s.fl, err = NewFileLogV3(s.filePath)
	s.Require().NoError(err)
}

func (s *V3FileTestSuite) requireJournalBoundary(active uint8) {
	header := fileHeaderV3{active: active, generation: 1, journalSize: journalSizeV3}
	slot := encodeFileHeaderV3(header)
	_, err := s.fl.file.file.WriteAt(slot[:], 0)
	s.Require().NoError(err)
	_, err = s.fl.file.file.WriteAt(slot[:], fileHeaderSlotSizeV3)
	s.Require().NoError(err)
	s.Require().NoError(openV3FileHandler(s.fl.file, header))
	record := newAppendTestRecord(1, 100)
	frame, err := appendFrameV3(nil, &record.SerializedRecord)
	s.Require().NoError(err)
	s.fl.file.writeOffset = s.fl.file.journalEnd - int64(len(frame)) - 1

	s.Require().NoError(s.fl.Write(&record.SerializedRecord))
	end := s.fl.file.writeOffset
	s.Require().Equal(s.fl.file.journalEnd-1, end)
	s.Require().ErrorIs(s.fl.Write(&record.SerializedRecord), ErrJournalFull)
	s.Require().Equal(end, s.fl.file.writeOffset)
	s.requireV3FileSizeOnDisk(s.filePath)
	var terminator [1]byte
	_, err = s.fl.file.file.ReadAt(terminator[:], end)
	s.Require().NoError(err)
	s.Require().Zero(terminator[0])
	other := journalOffsetV3(active ^ 1)
	_, err = s.fl.file.file.ReadAt(terminator[:], other)
	s.Require().NoError(err)
	s.Require().Zero(terminator[0])
}

func (s *V3FileTestSuite) requireRecords(expected ...*Record) {
	for _, want := range expected {
		var got SerializedRecord
		s.Require().NoError(s.fl.Read(&got))
		s.Require().Equal(want.SerializedRecord, got)
	}

	var got SerializedRecord
	s.Require().ErrorIs(s.fl.Read(&got), io.EOF)
}
