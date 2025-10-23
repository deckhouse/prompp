package head

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/headtest"
	"github.com/prometheus/prometheus/pp/go/relabeler/querier"
	"github.com/stretchr/testify/suite"
)

const (
	numberOfShards uint16 = 2

	maxSegmentSize uint32 = 1024

	headID                   = "test_head_id"
	transparentRelabelerName = "transparent_relabeler"

	unloadDataStorageInterval time.Duration = 100
)

var cfgs = []*config.InputRelabelerConfig{
	{
		Name: transparentRelabelerName,
		RelabelConfigs: []*cppbridge.RelabelConfig{
			{
				SourceLabels: []string{"__name__"},
				Regex:        ".*",
				Action:       cppbridge.Keep,
			},
		},
	},
}

type HeadLoadSuite struct {
	suite.Suite
	dataDir string
	ctx     context.Context
}

func TestHeadLoadSuite(t *testing.T) {
	suite.Run(t, new(HeadLoadSuite))
}

func (s *HeadLoadSuite) SetupTest() {
	s.dataDir = s.createDataDirectory()
}

func (s *HeadLoadSuite) createDataDirectory() string {
	dataDir := filepath.Join(s.T().TempDir(), "data")
	s.Require().NoError(os.MkdirAll(dataDir, 0o777))
	return dataDir
}

func (s *HeadLoadSuite) createHead(unloadDataStorageInterval time.Duration) (*Head, error) {
	return Create(
		headID,
		0,
		s.dataDir,
		cfgs,
		numberOfShards,
		maxSegmentSize,
		NoOpLastAppendedSegmentIDSetter{},
		prometheus.DefaultRegisterer,
		unloadDataStorageInterval,
	)
}

func (s *HeadLoadSuite) mustCreateHead(unloadDataStorageInterval time.Duration) *Head {
	h, err := s.createHead(unloadDataStorageInterval)
	s.Require().NoError(err)
	return h
}

func (s *HeadLoadSuite) loadHead(unloadDataStorageInterval time.Duration) (*Head, bool, error) {
	loadedHead, corrupted, _, err := Load(
		headID,
		0,
		s.dataDir,
		cfgs,
		numberOfShards,
		maxSegmentSize,
		NoOpLastAppendedSegmentIDSetter{},
		prometheus.DefaultRegisterer,
		unloadDataStorageInterval,
	)

	return loadedHead, corrupted, err
}

func (s *HeadLoadSuite) mustLoadHead(unloadDataStorageInterval time.Duration) *Head {
	loadedHead, corrupted, err := s.loadHead(unloadDataStorageInterval)

	s.Require().NoError(err)
	s.False(corrupted)

	return loadedHead
}

func (s *HeadLoadSuite) lockFileForCreation(fileName string) {
	s.Require().NoError(os.Remove(fileName))
	s.Require().NoError(os.Mkdir(fileName, 0o755))
}

func (s *HeadLoadSuite) appendTimeSeries(head *Head, timeSeries []headtest.TimeSeries) {
	headtest.MustAppendTimeSeries(&s.Suite, head, transparentRelabelerName, timeSeries)
}

func (s *HeadLoadSuite) query(head *Head, matcher *labels.Matcher, minT, maxT int64) (result []headtest.TimeSeries) {
	q := querier.NewQuerier(head, querier.NoOpShardedDeduplicatorFactory(), minT, maxT, nil, nil)
	defer func() { _ = q.Close() }()
	return headtest.TimeSeriesFromSeriesSet(q.Select(context.Background(), false, nil, matcher))
}

func (s *HeadLoadSuite) TestErrorCreateShardFileInOneShard() {
	// Arrange
	s.Require().NoError(os.Mkdir(getShardWalFilename(s.dataDir, 0), 0o777))

	// Act
	head, err := s.createHead(0)

	// Assert
	s.Require().Error(err)
	s.Nil(head)
}

func (s *HeadLoadSuite) TestErrorOpenShardFileInOneShard() {
	// Arrange
	sourceHead := s.mustCreateHead(0)
	sourceHead.Stop()
	s.NoError(sourceHead.Close())

	s.Require().NoError(os.Remove(getShardWalFilename(s.dataDir, 0)))

	// Act
	head, corrupted, err := s.loadHead(0)

	// Assert
	s.True(corrupted)
	s.Require().NoError(err)
	s.Nil(head.shards[0].unloadedDataStorage)
	s.Require().NoError(head.Close())
}

func (s *HeadLoadSuite) TestErrorOpenShardFileInAllShards() {
	// Arrange
	sourceHead := s.mustCreateHead(0)
	sourceHead.Stop()
	s.NoError(sourceHead.Close())

	s.Require().NoError(os.Remove(getShardWalFilename(s.dataDir, 0)))
	s.Require().NoError(os.Remove(getShardWalFilename(s.dataDir, 1)))

	// Act
	head, corrupted, err := s.loadHead(0)

	// Assert
	s.True(corrupted)
	s.Require().NoError(err)
	s.Nil(head.shards[0].unloadedDataStorage)
	s.Nil(head.shards[1].unloadedDataStorage)
	s.Require().NoError(head.Close())
}

func (s *HeadLoadSuite) TestLoadWithDisabledDataUnloading() {
	// Arrange
	sourceHead := s.mustCreateHead(0)

	series := []headtest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
				{Timestamp: 1, Value: 2},
				{Timestamp: 2, Value: 3},
			},
		},
	}
	s.appendTimeSeries(sourceHead, series)
	sourceHead.Stop()
	s.NoError(sourceHead.Close())

	// Act
	loadedHead := s.mustLoadHead(0)

	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "wal_metric")
	actual := s.query(loadedHead, matcher, 0, 2)
	err := loadedHead.Close()

	// Assert
	s.Nil(loadedHead.shards[0].unloadedDataStorage)
	s.Nil(loadedHead.shards[0].queriedSeriesStorage)
	s.Nil(loadedHead.shards[1].unloadedDataStorage)
	s.Nil(loadedHead.shards[1].queriedSeriesStorage)
	s.Equal(series, actual)
	s.Require().NoError(err)
}

func (s *HeadLoadSuite) TestAppendAfterLoad() {
	// Arrange
	sourceHead := s.mustCreateHead(0)
	series := []headtest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
				{Timestamp: 1, Value: 2},
				{Timestamp: 2, Value: 3},
			},
		},
	}
	s.appendTimeSeries(sourceHead, series)
	sourceHead.Stop()
	s.NoError(sourceHead.Close())

	// Act
	loadedHead := s.mustLoadHead(0)
	s.appendTimeSeries(loadedHead, []headtest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 3, Value: 4},
			},
		},
	})

	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "wal_metric")
	actual := s.query(loadedHead, matcher, 0, 3)
	err := loadedHead.Close()

	// Assert
	series[0].Samples = append(series[0].Samples, cppbridge.Sample{Timestamp: 3, Value: 4})
	s.Equal(series, actual)
	s.Require().NoError(err)
}

func (s *HeadLoadSuite) TestLoadWithEnabledDataUnloading() {
	// Arrange
	sourceHead := s.mustCreateHead(0)
	series1 := []headtest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
				{Timestamp: 1, Value: 2},
				{Timestamp: 2, Value: 3},
			},
		},
	}
	s.appendTimeSeries(sourceHead, series1)
	series2 := []headtest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1},
				{Timestamp: 101, Value: 2},
				{Timestamp: 102, Value: 3},
			},
		},
	}
	s.appendTimeSeries(sourceHead, series2)

	sourceHead.Stop()
	s.NoError(sourceHead.Close())

	// Act
	loadedHead := s.mustLoadHead(unloadDataStorageInterval)
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "wal_metric")
	actual := s.query(loadedHead, matcher, 0, 3)

	// Assert
	s.Equal(series1, actual)
	s.Require().NotNil(loadedHead.shards[0].unloadedDataStorage)
	s.Require().NotNil(loadedHead.shards[1].unloadedDataStorage)
	s.True(loadedHead.shards[0].unloadedDataStorage.storage.IsEmpty())
	s.True(loadedHead.shards[1].unloadedDataStorage.storage.IsEmpty())
	s.Require().NoError(loadedHead.Close())
}

func (s *HeadLoadSuite) TestLoadWithDataUnloading() {
	// Arrange
	sourceHead := s.mustCreateHead(unloadDataStorageInterval)
	series1 := []headtest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
				{Timestamp: 1, Value: 2},
				{Timestamp: 2, Value: 3},
			},
		},
	}
	s.appendTimeSeries(sourceHead, series1)
	series2 := []headtest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1},
				{Timestamp: 101, Value: 2},
				{Timestamp: 102, Value: 3},
			},
		},
	}
	s.appendTimeSeries(sourceHead, series2)

	sourceHead.UnloadUnusedSeriesData()
	sourceHead.Stop()
	s.NoError(sourceHead.Close())

	// Act
	loadedHead := s.mustLoadHead(unloadDataStorageInterval)
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "wal_metric")
	actual := s.query(loadedHead, matcher, 0, 3)

	// Assert
	s.Equal(series1, actual)
	s.NotNil(loadedHead.shards[0].unloadedDataStorage)
	s.NotNil(loadedHead.shards[1].unloadedDataStorage)
	s.False(loadedHead.shards[0].unloadedDataStorage.storage.IsEmpty())
	s.True(loadedHead.shards[1].unloadedDataStorage.storage.IsEmpty())
	s.Require().NoError(loadedHead.Close())
}

func (s *HeadLoadSuite) TestErrorDataUnloading() {
	// Arrange
	sourceHead := s.mustCreateHead(unloadDataStorageInterval)
	series1 := []headtest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
				{Timestamp: 1, Value: 2},
				{Timestamp: 2, Value: 3},
			},
		},
	}
	s.appendTimeSeries(sourceHead, series1)
	series2 := []headtest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1},
				{Timestamp: 101, Value: 2},
				{Timestamp: 102, Value: 3},
			},
		},
	}
	s.appendTimeSeries(sourceHead, series2)

	sourceHead.UnloadUnusedSeriesData()
	sourceHead.Stop()
	s.NoError(sourceHead.Close())

	// Act
	s.lockFileForCreation(getUnloadedDataStorageFilename(s.dataDir, 0))
	s.lockFileForCreation(getUnloadedDataStorageFilename(s.dataDir, 1))
	loadedHead, corrupted, err := s.loadHead(unloadDataStorageInterval)

	// Assert
	s.Require().NoError(err)
	s.True(corrupted)
	s.NotNil(loadedHead.shards[0].unloadedDataStorage)
	s.NotNil(loadedHead.shards[1].unloadedDataStorage)
	s.Require().NoError(loadedHead.Close())
}
