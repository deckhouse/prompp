package catalog

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
)

// HeadDirInspectorTestSuite checks filesystem metadata used to reconstruct lost catalog snapshots.
type HeadDirInspectorTestSuite struct {
	suite.Suite
	dataDir   string
	headID    string
	inspector HeadDirInspector
}

func TestHeadDirInspector(t *testing.T) {
	suite.Run(t, new(HeadDirInspectorTestSuite))
}

func (s *HeadDirInspectorTestSuite) SetupTest() {
	s.dataDir = s.T().TempDir()
	s.headID = uuid.NewString()
	s.inspector = NewHeadDirInspector(s.dataDir)
	s.Require().NoError(os.Mkdir(filepath.Join(s.dataDir, s.headID), 0o700))
}

func (s *HeadDirInspectorTestSuite) TestBirthTime() {
	before := time.Now()
	birth, err := s.inspector.BirthTime(s.headID)
	if err != nil {
		s.T().Skipf("filesystem does not expose directory birth time: %v", err)
	}

	s.WithinDuration(before, birth, time.Minute)
}

func (s *HeadDirInspectorTestSuite) TestEmptyDirectory() {
	count, err := s.inspector.NumberOfShards(s.headID)

	s.Require().NoError(err)
	s.Zero(count)
}

func (s *HeadDirInspectorTestSuite) TestMissingDirectory() {
	_, err := s.inspector.NumberOfShards(uuid.NewString())
	s.Require().ErrorIs(err, os.ErrNotExist)
	_, err = s.inspector.BirthTime(uuid.NewString())
	s.Require().Error(err)
}

func (s *HeadDirInspectorTestSuite) TestNumberOfShardsUsesMaximumIndex() {
	s.writeFile("shard_0.wal")
	s.writeFile("shard_3.wal")
	s.writeFile("shard_bad.wal")
	s.writeFile("shard_17.wal.tmp")
	s.writeFile("other")
	s.Require().NoError(os.Mkdir(filepath.Join(s.dataDir, s.headID, "shard_20.wal"), 0o700))

	count, err := s.inspector.NumberOfShards(s.headID)

	s.Require().NoError(err)
	s.Equal(uint16(4), count)
}

func (s *HeadDirInspectorTestSuite) TestRejectsUnrepresentableShardCount() {
	s.writeFile("shard_65535.wal")

	_, err := s.inspector.NumberOfShards(s.headID)

	s.Require().Error(err)
}

func (s *HeadDirInspectorTestSuite) writeFile(name string) {
	s.Require().NoError(os.WriteFile(filepath.Join(s.dataDir, s.headID, name), nil, 0o600))
}
