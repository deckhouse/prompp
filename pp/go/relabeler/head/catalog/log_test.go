package catalog

import (
	"encoding/hex"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"os"
	"path/filepath"
	"testing"
)

type FileLogTestSuite struct {
	suite.Suite
	records []*Record
}

func TestFileLog(t *testing.T) {
	suite.Run(t, new(FileLogTestSuite))
}

func (s *FileLogTestSuite) SetupSuite() {
	id1 := uuid.MustParse("fe52d991-fe22-41d9-9642-7e4d66d81a0c")
	var lastWrittenSegmentIDForID1 uint32 = 5
	id2 := uuid.MustParse("2d89cb33-9daa-4aea-9855-f844add5e3e4")
	id3 := uuid.MustParse("ec0c2898-9c42-449c-9a58-74bea665481c")

	s.records = []*Record{
		NewRecordWithData(id1, 1, 4, 5, 0, false, 0, StatusNew, &lastWrittenSegmentIDForID1),
		NewRecordWithData(id2, 2, 34, 420, 0, false, 0, StatusCorrupted, nil),
		NewRecordWithData(id3, 3, 25, 256, 0, false, 0, StatusPersisted, nil),
	}
}

func (s *FileLogTestSuite) TestMigrateV1ToV2() {
	logFileV1Name := filepath.Join(s.T().TempDir(), "v1.log")
	logFileV1, err := NewFileLogV1(logFileV1Name)
	require.NoError(s.T(), err)

	for _, record := range s.records {
		require.NoError(s.T(), logFileV1.Write(record))
	}
	require.NoError(s.T(), logFileV1.Close())

	fileContentIsEqual(s.T(), logFileV1Name, "testdata/headv1.log")

	logFile, err := NewFileLogV2(logFileV1Name)
	require.NoError(s.T(), err)
	require.NoError(s.T(), logFile.Close())
	fileContentIsEqual(s.T(), logFileV1Name, "testdata/headv2.log")
}

func (s *FileLogTestSuite) TestMigrateV2ToV3() {
	tmpDir := filepath.Join(s.T().TempDir(), "logtest")
	require.NoError(s.T(), os.CopyFS(tmpDir, os.DirFS("testdata")))
	logFilePath := filepath.Join(tmpDir, "headv2.log")
	logFile, err := NewFileLogV3(logFilePath)
	require.NoError(s.T(), err)
	require.NoError(s.T(), logFile.Close())
	fileContentIsEqual(s.T(), logFilePath, "testdata/headv3.log")
}

func (s *FileLogTestSuite) TestMigrateV3ToV2() {
	tmpDir := filepath.Join(s.T().TempDir(), "logtest")
	require.NoError(s.T(), os.CopyFS(tmpDir, os.DirFS("testdata")))
	logFilePath := filepath.Join(tmpDir, "headv3.log")
	logFile, err := NewFileLogV2(logFilePath)
	require.NoError(s.T(), err)
	require.NoError(s.T(), logFile.Close())
	fileContentIsEqual(s.T(), logFilePath, "testdata/headv2.log")
}

func (s *FileLogTestSuite) TestMigrateV1ToV3() {
	tmpDir := filepath.Join(s.T().TempDir(), "logtest")
	require.NoError(s.T(), os.CopyFS(tmpDir, os.DirFS("testdata")))
	logFilePath := filepath.Join(tmpDir, "headv1.log")
	logFile, err := NewFileLogV3(logFilePath)
	require.NoError(s.T(), err)
	require.NoError(s.T(), logFile.Close())
	fileContentIsEqual(s.T(), logFilePath, "testdata/headv3.log")
}

func fileContentIsEqual(t *testing.T, filePath1, filePath2 string) {
	data1, err := os.ReadFile(filePath1)
	require.NoError(t, err)
	t.Log(hex.EncodeToString(data1))
	data2, err := os.ReadFile(filePath2)
	require.NoError(t, err)
	t.Log(hex.EncodeToString(data1))
	require.Equal(t, data1, data2)
}
