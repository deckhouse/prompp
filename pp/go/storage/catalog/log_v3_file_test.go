package catalog

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

type V3FileTestSuite struct {
	suite.Suite
	filePath string
	fl       *FileLog
}

func TestV3File(t *testing.T) {
	suite.Run(t, new(V3FileTestSuite))
}

func (s *V3FileTestSuite) SetupTest() {
	s.filePath = filepath.Join(s.T().TempDir(), "head.log")
	var err error
	s.fl, err = NewFileLogV3(s.filePath)
	s.Require().NoError(err)
}

func (s *V3FileTestSuite) TearDownTest() {
	if s.fl != nil {
		if _, err := s.fl.file.file.Stat(); !errors.Is(err, os.ErrClosed) {
			s.Require().NoError(s.fl.Close())
		}
	}
}

func (s *V3FileTestSuite) TestCreateV3File_PreallocatedSize() {
	// Arrange
	filePath := s.filePath

	// Assert
	fl := s.fl
	var err error

	s.Require().Equal(fileSizeV3, fl.Size())

	info, err := os.Stat(filePath)
	s.Require().NoError(err)
	s.Require().Equal(int64(fileSizeV3), info.Size())

	firstByte := make([]byte, 1)
	_, err = s.readFileBytesAt(filePath, firstByte, 0)
	s.Require().NoError(err)
	s.Require().Equal(byte(LogFileVersionV3), firstByte[0])
}

func (s *V3FileTestSuite) TestReopenV3File_EmptyJournal() {
	// Arrange
	filePath := s.filePath
	fl := s.fl
	var err error
	s.Require().NoError(fl.Close())

	// Act
	reopened, err := NewFileLogV3(filePath)

	// Assert
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(reopened.Close()) })

	s.Require().Equal(fileSizeV3, reopened.Size())

	info, err := os.Stat(filePath)
	s.Require().NoError(err)
	s.Require().Equal(int64(fileSizeV3), info.Size())

	header, _, err := readFileHeaderV3(reopened.file.file)
	s.Require().NoError(err)
	s.Require().Zero(header.active)
	s.Require().Equal(uint64(1), header.generation)
	s.Require().Zero(header.activeUsed)

	s.Require().Equal(journalOffsetV3(0), reopened.file.readOffset)
	s.Require().Equal(journalOffsetV3(0), reopened.file.writeOffset)

	emptyJournal := make([]byte, journalSizeV3)
	for index := uint8(0); index < fileHeaderSlotsCount; index++ {
		journal := make([]byte, journalSizeV3)
		_, err = reopened.file.file.ReadAt(journal, journalOffsetV3(index))
		s.Require().NoError(err)
		s.Require().True(bytes.Equal(emptyJournal, journal), "journal %d is not empty", index)
	}

	sr := NewEmptyRecord()
	s.Require().ErrorIs(reopened.Read(&sr.SerializedRecord), io.EOF)
	s.Require().Equal(fileSizeV3, reopened.Size())
}

func (s *V3FileTestSuite) TestV3Header_CorruptedSlot() {
	// Arrange
	filePath := s.filePath
	fl := s.fl
	var err error
	s.Require().NoError(fl.Close())

	// Act: corrupt the generation field of slot 0, keeping the first byte intact.
	s.corruptV3HeaderSlot(filePath, fileHeaderGenerationOffsetV3)

	// Assert: the valid slot 1 wins.
	reopened, err := NewFileLogV3(filePath)
	s.Require().NoError(err)

	header, _, err := readFileHeaderV3(reopened.file.file)
	s.Require().NoError(err)
	s.Require().Zero(header.active)
	s.Require().Equal(uint64(1), header.generation)
	s.Require().NoError(reopened.Close())

	// Act: corrupt the same field of slot 1, so both slots fail the CRC check.
	s.corruptV3HeaderSlot(filePath, fileHeaderSlotSizeV3+fileHeaderGenerationOffsetV3)

	// Assert: opening fails with an explicit error.
	_, err = NewFileLogV3(filePath)
	s.Require().ErrorIs(err, ErrInvalidV3Header)
}

// corruptV3HeaderSlot overwrites one byte of the header slot at the given offset inside the slot.
func (s *V3FileTestSuite) corruptV3HeaderSlot(filePath string, offset int) {
	s.T().Helper()

	f, err := os.OpenFile(filePath, os.O_RDWR, logFilePerm)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(f.Close()) }()

	_, err = f.WriteAt([]byte{0xFF}, int64(offset))
	s.Require().NoError(err)
	s.Require().NoError(f.Sync())
}

func (s *V3FileTestSuite) readFileBytesAt(filePath string, p []byte, offset int64) (int, error) {
	s.T().Helper()

	f, err := os.Open(filePath) // #nosec G304 // it's meant to be that way
	s.Require().NoError(err)
	defer func() { s.Require().NoError(f.Close()) }()

	return f.ReadAt(p, offset)
}
