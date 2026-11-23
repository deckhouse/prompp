//go:build linux

package catalog

import (
	"fmt"
	"io"
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
	// Arrange
	path := filepath.Join(s.T().TempDir(), "head.log")
	fl, err := NewFileLogV3(path)
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(fl.Close()) })
	initial := newAppendTestRecord(1, 100)
	s.Require().NoError(fl.Write(&initial.SerializedRecord))
	offset, used := fl.file.writeOffset, fl.file.activeUsed
	failed := newAppendTestRecord(2, 200)
	s.requireV3AppendSyncFailure(fl, &failed.SerializedRecord)
	failed.fields = fieldStatus | fieldUpdatedAt | fieldSegmentsCount
	s.requireV3AppendSyncFailure(fl, &failed.SerializedRecord)
	s.Equal(offset, fl.file.writeOffset)
	s.Equal(used, fl.file.activeUsed)
	retry := SerializedRecord{id: failed.id, fields: fieldStatus, status: StatusRotated}

	// Act
	s.Require().NoError(fl.Write(&retry))
	s.Require().NoError(fl.Close())
	fl, err = NewFileLogV3(path)
	s.Require().NoError(err)

	// Assert
	var got SerializedRecord
	s.Require().NoError(fl.Read(&got))
	s.Equal(initial.SerializedRecord, got)
	s.Require().NoError(fl.Read(&got))
	s.Equal(retry, got)
	s.Require().ErrorIs(fl.Read(&got), io.EOF)
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

	err = fl.Write(&newAppendTestRecord(1, 100).SerializedRecord)

	s.Require().ErrorIs(err, syscall.EINVAL)
}

// requireV3AppendSyncFailure stages the frame on disk and uses /dev/null to fail the same append's fsync.
func (s *FileSyncSuite) requireV3AppendSyncFailure(fl *FileLog, record *SerializedRecord) {
	s.T().Helper()
	frame, err := appendFrameV3(nil, record)
	s.Require().NoError(err)
	s.Require().NoError(fl.file.WriteFixed(append(frame, recordVersionEndV3)))
	file := fl.file.file
	fl.file.file = s.file.file
	err = fl.Write(record)
	fl.file.file = file

	s.Require().ErrorIs(err, syscall.EINVAL)
}
