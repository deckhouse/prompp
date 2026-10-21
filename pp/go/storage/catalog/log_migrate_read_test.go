package catalog

import (
	"os"
	"path/filepath"

	"github.com/jonboulle/clockwork"
)

func (s *V3FileTestSuite) TestMigration_CompactedFallbackIsRenamed() {
	initial := newAppendTestRecord(1, 100)
	s.Require().NoError(s.fl.Write(&initial.SerializedRecord))
	s.Require().NoError(s.fl.Close())
	s.Require().NoError(os.Rename(s.filePath, s.filePath+".compacted"))
	s.Require().NoError(os.WriteFile(s.filePath, nil, logFilePerm))

	var err error
	s.fl, err = NewFileLogV3(s.filePath)
	s.Require().NoError(err)
	s.requireRecords(initial)

	_, err = os.Stat(s.filePath + ".compacted")
	s.Require().ErrorIs(err, os.ErrNotExist)
	s.Require().NoError(s.fl.Write(&newAppendTestRecord(2, 200).SerializedRecord))
}

func (s *V3FileTestSuite) TestMigration_FailedRewritePreservesSource() {
	initial := newAppendTestRecord(1, 100)
	s.Require().NoError(s.fl.Write(&initial.SerializedRecord))
	s.Require().NoError(s.fl.Close())

	before, err := os.ReadFile(s.filePath)
	s.Require().NoError(err)
	s.Require().NoError(os.Mkdir(s.filePath+".compacted", 0o700))

	s.fl, err = NewFileLogV2(s.filePath)
	s.Require().NoError(err)

	_, err = New(clockwork.NewFakeClock(), s.fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
	s.Require().Error(err)
	s.True(s.fl.NeedsRewrite())

	after, err := os.ReadFile(s.filePath)
	s.Require().NoError(err)
	s.Equal(before, after)
	s.Require().ErrorIs(s.fl.Write(&initial.SerializedRecord), ErrLogNeedsRewrite)
}

func (s *V3FileTestSuite) TestMigration_OpeningDoesNotChangeSource() {
	path := filepath.Join(s.T().TempDir(), "head.log")
	before, err := os.ReadFile("testdata/headv2.log")
	s.Require().NoError(err)
	s.Require().NoError(os.WriteFile(path, before, logFilePerm))

	fl, err := NewFileLogV3(path)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = fl.Close() })

	s.True(fl.NeedsRewrite())

	after, err := os.ReadFile(path)
	s.Require().NoError(err)
	s.Equal(before, after)
	s.Require().ErrorIs(fl.Write(&newAppendTestRecord(1, 100).SerializedRecord), ErrLogNeedsRewrite)
}
