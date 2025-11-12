package storage_test

import (
	"fmt"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/block"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var (
	defaultSortCatalogRecordsFunc = func(lhs, rhs *catalog.Record) bool {
		return lhs.CreatedAt() < rhs.CreatedAt()
	}
)

type UploadOrBuildHeadSuite struct {
	suite.Suite
	dataDir         string
	clock           *clockwork.FakeClock
	headIdGenerator catalog.IDGenerator
	catalog         *catalog.Catalog
	builder         *storage.Builder
	loader          *storage.Loader
}

func TestUploadOrBuildHeadSuite(t *testing.T) {
	suite.Run(t, new(UploadOrBuildHeadSuite))
}

func (s *UploadOrBuildHeadSuite) SetupTest() {
	dataDir, err := storagetest.CreateDataDirectory(s.T().TempDir())
	require.NoError(s.T(), err)
	s.dataDir = dataDir

	s.clock = clockwork.NewFakeClockAt(time.Now())
	s.headIdGenerator = catalog.DefaultIDGenerator{}
	s.createCatalog()
	s.createBuilder()
	s.createLoader()
}

func (s *UploadOrBuildHeadSuite) createCatalog() {
	var err error
	s.catalog, err = storagetest.CreateCatalog(s.clock, filepath.Join(s.dataDir, "catalog.log"), s.headIdGenerator)
	require.NoError(s.T(), err)
}

func createBuilder(catalog *catalog.Catalog, dataDir string, unloadDataStorageInterval time.Duration) *storage.Builder {
	return storage.NewBuilder(catalog, dataDir, storagetest.MaxSegmentSize, prometheus.DefaultRegisterer, unloadDataStorageInterval)
}

func (s *UploadOrBuildHeadSuite) createBuilder() {
	s.builder = createBuilder(s.catalog, s.dataDir, storagetest.UnloadDataStorageInterval)
}

func (s *UploadOrBuildHeadSuite) createLoader() {
	s.loader = storage.NewLoader(s.dataDir, storagetest.MaxSegmentSize, prometheus.DefaultRegisterer, storagetest.UnloadDataStorageInterval)
}

func (s *UploadOrBuildHeadSuite) createHead() (*storage.Head, error) {
	return s.builder.Build(0, storagetest.NumberOfShards)
}

func (s *UploadOrBuildHeadSuite) TestUploadOrBuildHeadSuccess() {
	// Arrange
	createdHead, err := s.createHead()
	require.NoError(s.T(), err)

	// Act
	loadedHead, err := storage.UploadOrBuildHead(s.clock, s.catalog, s.builder, s.loader, block.DefaultBlockDuration, storagetest.NumberOfShards)
	headRecords := s.catalog.List(nil, defaultSortCatalogRecordsFunc)

	// Assert
	require.NoError(s.T(), err)
	require.Equal(s.T(), createdHead.ID(), loadedHead.ID())
	require.Equal(s.T(), uint64(0), loadedHead.Generation())
	require.NoError(s.T(), createdHead.Close())
	require.NoError(s.T(), loadedHead.Close())

	require.Len(s.T(), headRecords, 1)
}

func (s *UploadOrBuildHeadSuite) TestUploadOrBuildHeadCorrupted() {
	// Arrange
	createdHead, err := s.createHead()
	require.NoError(s.T(), err)
	require.NoError(s.T(), createdHead.Close())
	createdHeadDir := filepath.Join(s.dataDir, createdHead.ID())
	require.NoError(s.T(), os.RemoveAll(createdHeadDir))
	s.clock.Advance(time.Hour)

	// Act
	builtHead, err := storage.UploadOrBuildHead(s.clock, s.catalog, s.builder, s.loader, block.DefaultBlockDuration, storagetest.NumberOfShards)
	headRecords := s.catalog.List(nil, defaultSortCatalogRecordsFunc)

	// Assert
	require.Nil(s.T(), err)
	require.NotEqual(s.T(), builtHead.ID(), createdHead.ID())
	require.Equal(s.T(), uint64(1), builtHead.Generation())
	require.NoError(s.T(), builtHead.Close())

	require.Len(s.T(), headRecords, 2)
	require.True(s.T(), headRecords[0].Corrupted())
}

func (s *UploadOrBuildHeadSuite) fixWalEncoderVersion(headDir string, numberOfShards uint16, encoderVersion uint8) {
	for i := uint16(0); i < numberOfShards; i++ {
		file, err := os.OpenFile(filepath.Join(headDir, fmt.Sprintf("shard_%d.wal", i)), os.O_RDWR|os.O_TRUNC, 0666)
		require.NoError(s.T(), err)
		_, err = writer.WriteHeader(file, wal.FileFormatVersion, encoderVersion)
		require.NoError(s.T(), err)
		require.NoError(s.T(), file.Close())
	}
}

func (s *UploadOrBuildHeadSuite) TestUploadOrBuildHeadOutdatedEncoderVersion() {
	// Arrange
	createdHead, err := s.createHead()
	require.NoError(s.T(), err)
	require.NoError(s.T(), createdHead.Close())
	createdHeadDir := filepath.Join(s.dataDir, createdHead.ID())
	s.fixWalEncoderVersion(createdHeadDir, storagetest.NumberOfShards, cppbridge.EncodersVersion()-1)
	s.clock.Advance(time.Hour)

	// Act
	builtHead, err := storage.UploadOrBuildHead(s.clock, s.catalog, s.builder, s.loader, block.DefaultBlockDuration, storagetest.NumberOfShards)
	headRecords := s.catalog.List(nil, defaultSortCatalogRecordsFunc)

	// Assert
	require.Nil(s.T(), err)
	require.NotEqual(s.T(), builtHead.ID(), createdHead.ID())
	require.Equal(s.T(), uint64(1), builtHead.Generation())
	require.NoError(s.T(), builtHead.Close())

	require.Len(s.T(), headRecords, 2)
	require.False(s.T(), headRecords[0].Corrupted())
}
