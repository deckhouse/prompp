package storage_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/stretchr/testify/suite"
)

const (
	numberOfShards uint16 = 2

	maxSegmentSize uint32 = 1024

	unloadDataStorageInterval time.Duration = 100
)

type idGeneratorStub struct {
	uuid uuid.UUID
}

func newIdGeneratorStub() *idGeneratorStub {
	return &idGeneratorStub{
		uuid: uuid.New(),
	}
}

func (g *idGeneratorStub) Generate() uuid.UUID {
	return g.uuid
}

func (g *idGeneratorStub) last() string {
	return g.uuid.String()
}

type HeadLoadSuite struct {
	suite.Suite
	dataDir         string
	clock           clockwork.Clock
	headIdGenerator *idGeneratorStub
	catalog         *catalog.Catalog
}

func TestHeadLoadSuite(t *testing.T) {
	suite.Run(t, new(HeadLoadSuite))
}

func (s *HeadLoadSuite) SetupTest() {
	s.dataDir = s.createDataDirectory()

	s.clock = clockwork.NewFakeClockAt(time.Now())
	s.headIdGenerator = newIdGeneratorStub()
	s.createCatalog()
}

func (s *HeadLoadSuite) createDataDirectory() string {
	dataDir := filepath.Join(s.T().TempDir(), "data")
	s.Require().NoError(os.MkdirAll(dataDir, os.ModeDir))
	return dataDir
}

func (s *HeadLoadSuite) createCatalog() {
	l, err := catalog.NewFileLogV2(filepath.Join(s.dataDir, "catalog.log"))
	s.Require().NoError(err)

	s.catalog, err = catalog.New(
		s.clock,
		l,
		s.headIdGenerator,
		catalog.DefaultMaxLogFileSize,
		nil,
	)
	s.Require().NoError(err)
}

func (s *HeadLoadSuite) headDir() string {
	return filepath.Join(s.dataDir, s.headIdGenerator.last())
}

func (s *HeadLoadSuite) createHead(unloadDataStorageInterval time.Duration) (*storage.Head, error) {
	return storage.NewBuilder(
		s.catalog,
		s.dataDir,
		maxSegmentSize,
		prometheus.DefaultRegisterer,
		unloadDataStorageInterval,
	).Build(0, numberOfShards)
}

func (s *HeadLoadSuite) mustCreateHead(unloadDataStorageInterval time.Duration) *storage.Head {
	h, err := s.createHead(unloadDataStorageInterval)
	s.Require().NoError(err)

	s.catalog.SetStatus(h.ID(), catalog.StatusActive)

	return h
}

func (s *HeadLoadSuite) loadHead(unloadDataStorageInterval time.Duration) (*storage.Head, storage.LoadResultType) {
	record, err := s.catalog.Get(s.headIdGenerator.last())
	s.Require().NoError(err)

	return storage.NewLoader(s.dataDir, maxSegmentSize, prometheus.DefaultRegisterer, unloadDataStorageInterval).Load(record, 0)
}

func (s *HeadLoadSuite) mustLoadHead(unloadDataStorageInterval time.Duration) *storage.Head {
	loadedHead, result := s.loadHead(unloadDataStorageInterval)
	s.Equal(storage.LoadResultTypeSuccess, result)

	return loadedHead
}

func (s *HeadLoadSuite) lockFileForCreation(fileName string) {
	s.Require().NoError(os.RemoveAll(fileName))
	s.Require().NoError(os.Mkdir(fileName, os.ModeDir))
}

func (s *HeadLoadSuite) appendTimeSeries(head *storage.Head, timeSeries []storagetest.TimeSeries) {
	storagetest.MustAppendTimeSeries(&s.Suite, head, timeSeries)
}

func (*HeadLoadSuite) shards(head *storage.Head) (result []*shard.Shard) {
	for sd := range head.RangeShards() {
		result = append(result, sd)
	}

	return result
}

func (s *HeadLoadSuite) TestErrorCreateShardFileInOneShard() {
	// Arrange
	s.Require().NoError(os.Mkdir(s.headDir(), 0), os.ModeDir)
	s.lockFileForCreation(storage.GetShardWalFilename(s.headDir(), 0))

	// Act
	head, err := s.createHead(0)

	// Assert
	s.Require().Error(err)
	s.Nil(head)
}

func (s *HeadLoadSuite) TestErrorOpenShardFileInOneShard() {
	// Arrange
	sourceHead := s.mustCreateHead(0)
	s.NoError(sourceHead.Close())

	s.Require().NoError(os.Remove(storage.GetShardWalFilename(s.headDir(), 0)))

	// Act
	head, result := s.loadHead(0)

	// Assert
	s.Equal(storage.LoadResultTypeCorrupted, result)
	s.Nil(s.shards(head)[0].UnloadedDataStorage())
	s.Require().NoError(head.Close())
}

func (s *HeadLoadSuite) TestErrorOpenShardFileInAllShards() {
	// Arrange
	sourceHead := s.mustCreateHead(0)
	s.NoError(sourceHead.Close())

	s.Require().NoError(os.Remove(storage.GetShardWalFilename(s.headDir(), 0)))
	s.Require().NoError(os.Remove(storage.GetShardWalFilename(s.headDir(), 1)))

	// Act
	head, result := s.loadHead(0)

	// Assert
	s.Equal(storage.LoadResultTypeCorrupted, result)
	s.Nil(s.shards(head)[0].UnloadedDataStorage())
	s.Nil(s.shards(head)[1].UnloadedDataStorage())
	s.Require().NoError(head.Close())
}

func (s *HeadLoadSuite) TestLoadWithDisabledDataUnloading() {
	// Arrange
	sourceHead := s.mustCreateHead(0)
	s.appendTimeSeries(sourceHead, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
				{Timestamp: 1, Value: 2},
				{Timestamp: 2, Value: 3},
			},
		},
	})
	s.Require().NoError(sourceHead.Close())

	// Act
	loadedHead := s.mustLoadHead(0)

	queryResult := s.shards(loadedHead)[0].DataStorage().Query(cppbridge.HeadDataStorageQuery{
		StartTimestampMs: 0,
		EndTimestampMs:   2,
		LabelSetIDs:      []uint32{0},
	})
	err := loadedHead.Close()

	// Assert
	s.Require().NoError(err)
	s.Nil(s.shards(loadedHead)[0].UnloadedDataStorage())
	s.Nil(s.shards(loadedHead)[0].QueriedSeriesStorage())
	s.Nil(s.shards(loadedHead)[1].UnloadedDataStorage())
	s.Nil(s.shards(loadedHead)[1].QueriedSeriesStorage())
	s.Equal(cppbridge.DataStorageQueryStatusSuccess, queryResult.Status)
	s.Equal(storagetest.SamplesMap{
		0: []cppbridge.Sample{
			{Timestamp: 0, Value: 1},
			{Timestamp: 1, Value: 2},
			{Timestamp: 2, Value: 3},
		},
	}, storagetest.GetSamplesFromSerializedData(queryResult.SerializedData))
	s.Equal([]cppbridge.Labels{
		{{Name: "__name__", Value: "wal_metric"}},
	}, s.shards(loadedHead)[0].LSS().Target().GetLabelSets([]uint32{0}).LabelsSets())
}

func (s *HeadLoadSuite) TestAppendAfterLoad() {
	// Arrange
	sourceHead := s.mustCreateHead(0)
	s.appendTimeSeries(sourceHead, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
				{Timestamp: 1, Value: 2},
				{Timestamp: 2, Value: 3},
			},
		},
	})
	s.Require().NoError(sourceHead.Close())

	// Act
	loadedHead := s.mustLoadHead(0)
	s.appendTimeSeries(loadedHead, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 3, Value: 4},
			},
		},
	})

	queryResult := s.shards(loadedHead)[0].DataStorage().Query(cppbridge.HeadDataStorageQuery{
		StartTimestampMs: 0,
		EndTimestampMs:   4,
		LabelSetIDs:      []uint32{0},
	})

	err := loadedHead.Close()

	// Assert
	s.Require().NoError(err)
	s.Equal(cppbridge.DataStorageQueryStatusSuccess, queryResult.Status)
	s.Equal(storagetest.SamplesMap{
		0: []cppbridge.Sample{
			{Timestamp: 0, Value: 1},
			{Timestamp: 1, Value: 2},
			{Timestamp: 2, Value: 3},
			{Timestamp: 3, Value: 4},
		},
	}, storagetest.GetSamplesFromSerializedData(queryResult.SerializedData))
	s.Equal([]cppbridge.Labels{
		{{Name: "__name__", Value: "wal_metric"}},
	}, s.shards(loadedHead)[0].LSS().Target().GetLabelSets([]uint32{0}).LabelsSets())
}

func (s *HeadLoadSuite) TestLoadWithEnabledDataUnloading() {
	// Arrange
	sourceHead := s.mustCreateHead(0)
	s.appendTimeSeries(sourceHead, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
				{Timestamp: 1, Value: 2},
				{Timestamp: 2, Value: 3},
			},
		},
	})
	s.appendTimeSeries(sourceHead, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1},
				{Timestamp: 101, Value: 2},
				{Timestamp: 102, Value: 3},
			},
		},
	})

	s.Require().NoError(sourceHead.Close())

	// Act
	loadedHead := s.mustLoadHead(unloadDataStorageInterval)

	// Assert
	s.Require().NotNil(s.shards(loadedHead)[0].UnloadedDataStorage())
	s.Require().NotNil(s.shards(loadedHead)[1].UnloadedDataStorage())
	s.True(s.shards(loadedHead)[0].UnloadedDataStorage().IsEmpty())
	s.True(s.shards(loadedHead)[1].UnloadedDataStorage().IsEmpty())
	s.Require().NoError(loadedHead.Close())
}

func (s *HeadLoadSuite) TestLoadWithDataUnloading() {
	// Arrange
	sourceHead := s.mustCreateHead(unloadDataStorageInterval)
	s.appendTimeSeries(sourceHead, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
				{Timestamp: 1, Value: 2},
				{Timestamp: 2, Value: 3},
			},
		},
	})
	s.appendTimeSeries(sourceHead, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1},
				{Timestamp: 101, Value: 2},
				{Timestamp: 102, Value: 3},
			},
		},
	})

	s.Require().NoError(services.UnloadUnusedSeriesDataWithHead(sourceHead))
	s.Require().NoError(sourceHead.Close())

	// Act
	loadedHead := s.mustLoadHead(unloadDataStorageInterval)

	// Assert
	s.NotNil(s.shards(loadedHead)[0].UnloadedDataStorage())
	s.NotNil(s.shards(loadedHead)[1].UnloadedDataStorage())
	s.False(s.shards(loadedHead)[0].UnloadedDataStorage().IsEmpty())
	s.True(s.shards(loadedHead)[1].UnloadedDataStorage().IsEmpty())
	s.Require().NoError(loadedHead.Close())
}

func (s *HeadLoadSuite) TestErrorDataUnloading() {
	// Arrange
	sourceHead := s.mustCreateHead(unloadDataStorageInterval)
	s.appendTimeSeries(sourceHead, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
				{Timestamp: 1, Value: 2},
				{Timestamp: 2, Value: 3},
			},
		},
	})
	s.appendTimeSeries(sourceHead, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1},
				{Timestamp: 101, Value: 2},
				{Timestamp: 102, Value: 3},
			},
		},
	})

	s.Require().NoError(services.UnloadUnusedSeriesDataWithHead(sourceHead))
	s.Require().NoError(sourceHead.Close())

	// Act
	s.lockFileForCreation(storage.GetUnloadedDataStorageFilename(s.headDir(), 0))
	s.lockFileForCreation(storage.GetUnloadedDataStorageFilename(s.headDir(), 1))
	loadedHead, result := s.loadHead(unloadDataStorageInterval)

	// Assert
	s.Equal(storage.LoadResultTypeCorrupted, result)
	s.NotNil(s.shards(loadedHead)[0].UnloadedDataStorage())
	s.NotNil(s.shards(loadedHead)[1].UnloadedDataStorage())
	s.Require().NoError(loadedHead.Close())
}
