package catalog

import (
	"io"
	"os"
	"syscall"
)

func (s *V3FileTestSuite) TestV3WritesNeedNoDiskSpace() {
	// Arrange: creation preallocates the fixed size — the only operation that needs disk space.
	filePath := s.filePath
	fl := s.fl
	var err error

	s.requireV3FileSizeOnDisk(filePath)

	// Act: append using the production bounds checks without growing the file.

	expected := make([]*Record, 5)
	for i := range expected {
		expected[i] = newAppendTestRecord(uint16(i+1), int64(100+i))
		s.Require().NoError(fl.Write(&expected[i].SerializedRecord), "append %d", i)
		s.Require().Equal(fileSizeV3, fl.Size(), "append %d", i)
		s.requireV3FileSizeOnDisk(filePath)
	}

	// Act: closing updates the other header slot without growing the file.
	s.Require().NoError(fl.Close())
	s.requireV3FileSizeOnDisk(filePath)

	reopened, err := NewFileLogV3(filePath)
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(reopened.Close()) })

	// Assert: every record reads back in order, then a clean end, size unchanged throughout.
	for i, want := range expected {
		var got SerializedRecord
		s.Require().NoError(reopened.Read(&got), "record %d", i)
		s.Require().Equal(want.SerializedRecord, got, "record %d", i)
	}

	var got SerializedRecord
	s.Require().ErrorIs(reopened.Read(&got), io.EOF)
	s.requireV3FileSizeOnDisk(filePath)

	// Negative control: a positioned write past the accounted size is refused with ENOSPC.
	_, err = reopened.file.writeAt([]byte{0xAA}, int64(fileSizeV3))
	s.Require().ErrorIs(err, syscall.ENOSPC)

	// Negative control through the production append path: WriteFixed at the end refuses too.
	writeOffset := reopened.file.writeOffset
	reopened.file.writeOffset = int64(fileSizeV3)
	s.Require().ErrorIs(reopened.file.WriteFixed([]byte{0xAA}), syscall.ENOSPC)
	reopened.file.writeOffset = writeOffset

	// A write ending exactly at fileSizeV3 stays an overwrite and goes through.
	n, err := reopened.file.writeAt([]byte{0}, int64(fileSizeV3)-1)
	s.Require().NoError(err)
	s.Require().Equal(1, n)

	s.requireV3FileSizeOnDisk(filePath)
}

// requireV3FileSizeOnDisk asserts that the log file version 3 keeps its fixed size on disk.
func (s *V3FileTestSuite) requireV3FileSizeOnDisk(filePath string) {
	s.T().Helper()

	info, err := os.Stat(filePath)
	s.Require().NoError(err)
	s.Require().Equal(int64(fileSizeV3), info.Size())
}
