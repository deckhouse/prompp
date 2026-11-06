package catalog

import (
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func (s *FileLogReplaceTestSuite) TestCreateSwap_DirectorySyncFailureRemovesEmptyV3() {
	s.requireFailedV3Creation(preallocateV3File, func(dir string) error {
		s.Equal(filepath.Dir(s.filePath), dir)
		file, version, err := loadFile(s.filePath + ".compacted")
		s.Require().NoError(err)
		s.Equal(LogFileVersionV3, version)
		reader := recordReader{file: file}
		var record SerializedRecord
		s.Require().ErrorIs(reader.Read(&record), io.EOF)
		s.Require().NoError(file.Close())

		return syscall.EIO
	}, syscall.EIO)

	appended := newAppendTestRecord(3, 300)
	s.Require().NoError(s.fl.Write(&appended.SerializedRecord))
	s.reopen(LogFileVersionV2)
	s.requireRecords(s.initial, appended)
}

func (s *FileLogReplaceTestSuite) TestCreateSwap_InitializationFailureRemovesFile() {
	s.requireFailedV3Creation(func(file *os.File) error {
		if err := preallocateV3File(file); err != nil {
			return err
		}

		return file.Close()
	}, syncDir, os.ErrClosed)
}

func (s *FileLogReplaceTestSuite) TestCreateSwap_OpenFailurePreservesDirectory() {
	swapPath := s.filePath + ".compacted"
	s.Require().NoError(os.Mkdir(swapPath, 0o700))

	file, err := createSwapFile(swapPath, LogFileVersionV3, NewEncoderV3())

	s.Require().Error(err)
	s.Nil(file)
	info, err := os.Stat(swapPath)
	s.Require().NoError(err)
	s.True(info.IsDir())
	s.reopen(LogFileVersionV2)
	s.requireRecords(s.initial)
}

func (s *FileLogReplaceTestSuite) TestCreateSwap_PreallocationFailureRemovesFile() {
	for _, expectedErr := range []error{syscall.ENOSPC, syscall.EIO} {
		s.Run(expectedErr.Error(), func() {
			s.requireFailedV3Creation(func(file *os.File) error {
				if err := file.Truncate(int64(fileSizeV3 / 2)); err != nil {
					return err
				}

				return expectedErr
			}, syncDir, expectedErr)
		})
	}
}

func (s *FileLogReplaceTestSuite) TestRewrite_OverflowRemovesSwap() {
	s.reopen(LogFileVersionV3)
	records := make([]*SerializedRecord, 100000)
	for index := range records {
		records[index] = &s.initial.SerializedRecord
	}

	s.Require().ErrorIs(s.fl.ReWrite(records...), ErrJournalFull)

	_, err := os.Stat(s.filePath + ".compacted")
	s.Require().ErrorIs(err, os.ErrNotExist)
	s.True(s.fl.NeedsRewrite())
	s.reopen(LogFileVersionV2)
	s.requireRecords(s.initial)
}

func (s *FileLogReplaceTestSuite) requireFailedV3Creation(
	preallocate func(*os.File) error,
	syncDirectory func(string) error,
	expectedErr error,
) {
	s.T().Helper()
	before, err := os.ReadFile(s.filePath)
	s.Require().NoError(err)
	oldFile := s.fl.file
	var createdFile *os.File
	swapPath := s.filePath + ".compacted"

	file, err := createV3FileHandler(swapPath, func(file *os.File) error {
		createdFile = file

		return preallocate(file)
	}, syncDirectory)

	s.Require().ErrorIs(err, expectedErr)
	s.Nil(file)
	s.Require().NotNil(createdFile)
	_, err = createdFile.Stat()
	s.Require().ErrorIs(err, os.ErrClosed)
	_, err = os.Stat(swapPath)
	s.Require().ErrorIs(err, os.ErrNotExist)
	after, err := os.ReadFile(s.filePath)
	s.Require().NoError(err)
	s.Equal(before, after)
	s.Same(oldFile, s.fl.file)
	s.reopen(LogFileVersionV2)
	s.requireRecords(s.initial)
}
