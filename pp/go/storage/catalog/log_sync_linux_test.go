//go:build linux

package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/suite"
)

// FileSyncSuite uses /dev/null, whose writes succeed but whose fsync fails, to check durability boundaries.
type FileSyncSuite struct {
	suite.Suite

	file *FileHandler
}

func TestFileSync(t *testing.T) {
	suite.Run(t, new(FileSyncSuite))
}

func (s *FileSyncSuite) SetupTest() {
	var err error
	s.file, err = NewFileHandlerWithOpts(os.DevNull, os.O_RDWR, 0)
	s.Require().NoError(err)
}

func (s *FileSyncSuite) TearDownTest() {
	s.Require().NoError(s.file.Close())
}

func (s *FileSyncSuite) TestFileHandlerWriteDoesNotSync() {
	n, err := s.file.Write([]byte("record"))

	s.Require().NoError(err)
	s.Equal(len("record"), n)
	s.Require().ErrorIs(s.file.Sync(), syscall.EINVAL)
}

func (s *FileSyncSuite) TestFileHandlerWriteFixedDoesNotSync() {
	s.file.fixedSize = true
	s.file.size = fileSizeV3

	err := s.file.WriteFixed([]byte("record"))

	s.Require().NoError(err)
	s.Zero(s.file.writeOffset)
	s.Require().ErrorIs(s.file.Sync(), syscall.EINVAL)
}

func (s *FileSyncSuite) TestFileLogWriteSyncFailureV1() {
	s.requireAppendSyncFailure(LogFileVersionV1)
}

func (s *FileSyncSuite) TestFileLogWriteSyncFailureV2() {
	s.requireAppendSyncFailure(LogFileVersionV2)
}

func (s *FileSyncSuite) TestFileLogWriteSyncFailureV3() {
	s.requireAppendSyncFailure(LogFileVersionV3)
}

func (s *FileSyncSuite) TestNewLegacyLogSyncFailureRemovesFile() {
	for _, version := range []uint64{LogFileVersionV1, LogFileVersionV2} {
		s.Run(fmt.Sprintf("v%d", version), func() {
			path := filepath.Join(s.T().TempDir(), "head.log")
			s.Require().NoError(os.Symlink(os.DevNull, path))

			fl, err := NewFileLog(path, version)

			s.Require().ErrorIs(err, syscall.EINVAL)
			s.Nil(fl)
			_, err = os.Lstat(path)
			s.Require().ErrorIs(err, os.ErrNotExist)
		})
	}
}

func (s *FileSyncSuite) TestNewV1HeaderSyncFailure() {
	path := filepath.Join(s.T().TempDir(), "head.log")
	s.Require().NoError(os.Symlink(os.DevNull, path))

	fl, err := NewFileLogV1(path)

	s.Require().ErrorIs(err, syscall.EINVAL)
	s.Nil(fl)
}

func (s *FileSyncSuite) requireAppendSyncFailure(version uint64) {
	s.T().Helper()
	encoder, _, err := codecsByVersion(version)
	s.Require().NoError(err)
	fl := &FileLog{version: version, targetVersion: version, encoder: encoder, file: s.file}

	if version == LogFileVersionV3 {
		s.file.fixedSize = true
		s.file.size = fileSizeV3
		s.file.journalStart = fileHeaderAreaSizeV3
		s.file.journalEnd = fileHeaderAreaSizeV3 + journalSizeV3
		s.file.writeOffset = s.file.journalStart
	}

	err = fl.Write(&newAppendTestRecord(1, 100).SerializedRecord)

	s.Require().ErrorIs(err, syscall.EINVAL)

	if version == LogFileVersionV3 {
		s.Equal(s.file.journalStart, s.file.writeOffset)
		s.Zero(s.file.activeUsed)
	}
}
