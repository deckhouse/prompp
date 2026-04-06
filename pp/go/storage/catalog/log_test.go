package catalog_test

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/storage/catalog"
)

type FileLogTestSuite struct {
	suite.Suite
	records []*catalog.Record
}

func TestFileLog(t *testing.T) {
	suite.Run(t, new(FileLogTestSuite))
}

func (s *FileLogTestSuite) SetupSuite() {
	id1 := uuid.MustParse("fe52d991-fe22-41d9-9642-7e4d66d81a0c")
	var lastWrittenSegmentIDForID1 uint32 = 5
	id2 := uuid.MustParse("2d89cb33-9daa-4aea-9855-f844add5e3e4")
	id3 := uuid.MustParse("ec0c2898-9c42-449c-9a58-74bea665481c")

	s.records = []*catalog.Record{
		catalog.NewRecordWithData(id1, 1, 4, 5, 0, false, 0, catalog.StatusNew, &lastWrittenSegmentIDForID1),
		catalog.NewRecordWithData(id2, 2, 34, 420, 0, false, 0, catalog.StatusCorrupted, nil),
		catalog.NewRecordWithData(id3, 3, 25, 256, 0, false, 0, catalog.StatusPersisted, nil),
	}
}

func (s *FileLogTestSuite) TestMigrateV1ToV2() {
	logFileV1Name := filepath.Join(s.T().TempDir(), "v1.log")
	logFileV1, err := catalog.NewFileLogV1(logFileV1Name)
	s.Require().NoError(err)

	for _, record := range s.records {
		s.Require().NoError(logFileV1.Write(&record.SerializedRecord))
	}
	s.Require().NoError(logFileV1.Close())

	fileContentIsEqual(s, logFileV1Name, "testdata/headv1.log")

	logFile, err := catalog.NewFileLogV2(logFileV1Name)
	s.Require().NoError(err)
	s.Require().NoError(logFile.Close())

	fileContentIsEqual(s, logFileV1Name, "testdata/headv2.log")
}

func (s *FileLogTestSuite) TestMigrateV2ToV3() {
	tmpDir := filepath.Join(s.T().TempDir(), "logtest")
	s.Require().NoError(os.CopyFS(tmpDir, os.DirFS("testdata")))

	logFilePath := filepath.Join(tmpDir, "headv2.log")
	logFile, err := catalog.NewFileLogV3(logFilePath)
	s.Require().NoError(err)
	s.Require().NoError(logFile.Close())

	fileContentIsEqual(s, logFilePath, "testdata/headv3.log")
}

func (s *FileLogTestSuite) TestMigrateV3ToV2() {
	tmpDir := filepath.Join(s.T().TempDir(), "logtest")
	s.Require().NoError(os.CopyFS(tmpDir, os.DirFS("testdata")))

	logFilePath := filepath.Join(tmpDir, "headv3.log")
	logFile, err := catalog.NewFileLogV2(logFilePath)
	s.Require().NoError(err)
	s.Require().NoError(logFile.Close())

	fileContentIsEqual(s, logFilePath, "testdata/headv2.log")
}

func (s *FileLogTestSuite) TestMigrateV1ToV3() {
	tmpDir := filepath.Join(s.T().TempDir(), "logtest")
	s.Require().NoError(os.CopyFS(tmpDir, os.DirFS("testdata")))

	logFilePath := filepath.Join(tmpDir, "headv1.log")
	logFile, err := catalog.NewFileLogV3(logFilePath)
	s.Require().NoError(err)
	s.Require().NoError(logFile.Close())

	fileContentIsEqual(s, logFilePath, "testdata/headv3.log")
}

func fileContentIsEqual(s *FileLogTestSuite, filePath1, filePath2 string) {
	data1, err := os.ReadFile(filePath1)
	s.Require().NoError(err)
	s.T().Log(hex.EncodeToString(data1))

	data2, err := os.ReadFile(filePath2)
	s.Require().NoError(err)
	s.T().Log(hex.EncodeToString(data1))

	s.Require().Equal(data1, data2)
}
