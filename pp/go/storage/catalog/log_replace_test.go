package catalog

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/suite"
)

// FileLogReplaceTestSuite checks file ownership when a log replacement fails.
type FileLogReplaceTestSuite struct {
	suite.Suite
	filePath string
	fl       *FileLog
	initial  *Record
}

func TestFileLogReplace(t *testing.T) {
	suite.Run(t, new(FileLogReplaceTestSuite))
}

func (s *FileLogReplaceTestSuite) SetupTest() {
	s.filePath = filepath.Join(s.T().TempDir(), "head.log")
	var err error
	s.fl, err = NewFileLogV2(s.filePath)
	s.Require().NoError(err)
	s.initial = newAppendTestRecord(1, 100)
	s.Require().NoError(s.fl.Write(&s.initial.SerializedRecord))
}

func (s *FileLogReplaceTestSuite) TearDownTest() {
	if s.fl != nil {
		err := s.fl.Close()
		if !errors.Is(err, os.ErrClosed) {
			s.Require().NoError(err)
		}
	}
}

func (s *FileLogReplaceTestSuite) TestOpenFallback_DirectorySyncFailure() {
	s.Require().NoError(s.fl.Close())
	s.fl = nil
	before, err := os.ReadFile(s.filePath)
	s.Require().NoError(err)
	sourcePath := s.filePath + ".compacted"
	s.Require().NoError(os.Rename(s.filePath, sourcePath))
	s.Require().NoError(os.WriteFile(s.filePath, nil, logFilePerm))
	syncCalls := 0

	fl, err := openFileLog(s.filePath, sourcePath, LogFileVersionV2, func(dir string) error {
		s.Equal(filepath.Dir(s.filePath), dir)
		syncCalls++

		return syscall.EIO
	})

	s.Require().ErrorIs(err, syscall.EIO)
	s.Nil(fl)
	s.Equal(1, syncCalls)
	after, err := os.ReadFile(s.filePath)
	s.Require().NoError(err)
	s.Equal(before, after)
	_, err = os.Stat(sourcePath)
	s.Require().ErrorIs(err, os.ErrNotExist)

	s.fl, err = NewFileLogV2(s.filePath)
	s.Require().NoError(err)
	s.requireRecords(s.initial)
	appended := newAppendTestRecord(3, 300)
	s.Require().NoError(s.fl.Write(&appended.SerializedRecord))
	s.reopen(LogFileVersionV2)
	s.requireRecords(s.initial, appended)
}

func (s *FileLogReplaceTestSuite) TestRewrite_DirectorySyncFailureV2() {
	s.requireRewriteSyncFailure()
}

func (s *FileLogReplaceTestSuite) TestRewrite_DirectorySyncFailureV2ToV3() {
	s.reopen(LogFileVersionV3)
	s.requireRewriteSyncFailure()
}

func (s *FileLogReplaceTestSuite) TestRewrite_DirectorySyncFailureV3ToV2() {
	s.reopen(LogFileVersionV3)
	s.Require().NoError(s.fl.ReWrite(&s.initial.SerializedRecord))
	s.reopen(LogFileVersionV2)
	s.requireRewriteSyncFailure()
}

func (s *FileLogReplaceTestSuite) TestRewrite_RenameFailurePreservesWriter() {
	oldFile := s.fl.file
	originalPath := s.filePath + ".original"
	s.Require().NoError(os.Rename(s.filePath, originalPath))
	s.Require().NoError(os.Mkdir(s.filePath, 0o700))
	snapshot := newAppendTestRecord(2, 200)

	err := s.fl.ReWrite(&snapshot.SerializedRecord)

	s.Require().Error(err)
	s.Same(oldFile, s.fl.file)
	s.Require().NoError(os.Remove(s.filePath))
	s.Require().NoError(os.Rename(originalPath, s.filePath))
	appended := newAppendTestRecord(3, 300)
	s.Require().NoError(s.fl.Write(&appended.SerializedRecord))
	s.reopen(LogFileVersionV2)
	s.requireRecords(s.initial, appended)
}

func (s *FileLogReplaceTestSuite) reopen(targetVersion uint64) {
	s.T().Helper()
	err := s.fl.Close()
	if !errors.Is(err, os.ErrClosed) {
		s.Require().NoError(err)
	}

	s.fl, err = NewFileLog(s.filePath, targetVersion)
	s.Require().NoError(err)
}

func (s *FileLogReplaceTestSuite) requireRecords(expected ...*Record) {
	s.T().Helper()

	for _, want := range expected {
		var got SerializedRecord
		s.Require().NoError(s.fl.Read(&got))
		s.Equal(want.id, got.id)
		s.Equal(want.numberOfShards, got.numberOfShards)
		s.Equal(want.createdAt, got.createdAt)
	}

	var got SerializedRecord
	s.Require().ErrorIs(s.fl.Read(&got), io.EOF)
}

func (s *FileLogReplaceTestSuite) requireRewriteSyncFailure() {
	s.T().Helper()
	oldFile := s.fl.file
	targetVersion := s.fl.targetVersion
	snapshot := newAppendTestRecord(2, 200)
	var fileAtSync *FileHandler

	err := s.fl.rewriteV2([]*SerializedRecord{&snapshot.SerializedRecord}, func(dir string) error {
		s.Equal(filepath.Dir(s.filePath), dir)
		fileAtSync = s.fl.file

		return syscall.EIO
	})

	s.Require().ErrorIs(err, syscall.EIO)
	s.Require().NotSame(oldFile, fileAtSync, "the replacement must be adopted before syncing the directory")
	s.Same(fileAtSync, s.fl.file)
	s.Equal(targetVersion, s.fl.version)
	s.False(s.fl.NeedsRewrite())
	appended := newAppendTestRecord(3, 300)
	s.Require().ErrorIs(s.fl.Write(&appended.SerializedRecord), os.ErrClosed)
	_, err = oldFile.file.Stat()
	s.Require().ErrorIs(err, os.ErrClosed)
	_, err = s.fl.file.file.Stat()
	s.Require().ErrorIs(err, os.ErrClosed)
	_, err = os.Stat(s.filePath + ".compacted")
	s.Require().ErrorIs(err, os.ErrNotExist)

	s.reopen(targetVersion)
	s.requireRecords(snapshot)
	s.Require().NoError(s.fl.Write(&appended.SerializedRecord))
	s.reopen(targetVersion)
	s.requireRecords(snapshot, appended)
}
