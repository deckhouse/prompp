package querier_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/stretchr/testify/suite"
)

const (
	numberOfShards uint16 = 2

	maxSegmentSize uint32 = 1024
)

type Querier = querier.Querier[
	*task.Generic[*storage.PerGoroutineShard],
	*shard.DataStorage,
	*shard.LSS,
	*storage.PerGoroutineShard,
	*storage.HeadOnDisk,
]

type QuerierSuite struct {
	suite.Suite
	dataDir string
	context context.Context
	head    *storage.HeadOnDisk
}

func TestQuerierSuite(t *testing.T) {
	suite.Run(t, new(QuerierSuite))
}

func (s *QuerierSuite) SetupTest() {
	s.dataDir = s.createDataDirectory()
	s.context = context.Background()

	s.head = s.mustCreateHead(1)
}

func (s *QuerierSuite) createDataDirectory() string {
	dataDir := filepath.Join(s.T().TempDir(), "data")
	s.Require().NoError(os.MkdirAll(dataDir, os.ModeDir))
	return dataDir
}

func (s *QuerierSuite) mustCreateCatalog() *catalog.Catalog {
	l, err := catalog.NewFileLogV2(filepath.Join(s.dataDir, "catalog.log"))
	s.Require().NoError(err)

	c, err := catalog.New(
		clockwork.NewFakeClock(),
		l,
		&catalog.DefaultIDGenerator{},
		catalog.DefaultMaxLogFileSize,
		nil,
	)
	s.Require().NoError(err)

	return c
}

func (s *QuerierSuite) mustCreateHead(unloadDataStorageInterval time.Duration) *storage.HeadOnDisk {
	h, err := storage.NewBuilder(
		s.mustCreateCatalog(),
		s.dataDir,
		maxSegmentSize,
		prometheus.DefaultRegisterer,
		unloadDataStorageInterval,
	).Build(0, numberOfShards)
	s.Require().NoError(err)
	return h
}

func (s *QuerierSuite) appendTimeSeries(timeSeries []storagetest.TimeSeries) {
	storagetest.MustAppendTimeSeries(&s.Suite, s.head, timeSeries)
}

func (s *QuerierSuite) TestRangeQuery() {
	// Arrange
	timeSeries := []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test2"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 10},
			},
		},
	}
	s.appendTimeSeries(timeSeries)

	q := querier.NewQuerier[*task.Generic[*storage.PerGoroutineShard],
		*shard.DataStorage,
		*shard.LSS,
		*storage.PerGoroutineShard,
		*storage.HeadOnDisk,
	](s.head, querier.NewNoOpShardedDeduplicator, 0, 2, nil, nil)
	defer func() { _ = q.Close() }()
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")

	// Act
	seriesSet := q.Select(s.context, false, nil, matcher)

	// Assert
	s.Equal(timeSeries, storagetest.TimeSeriesFromSeriesSet(seriesSet))
}

func (s *QuerierSuite) TestRangeQueryWithoutMatching() {
	// Arrange
	timeSeries := []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
			},
		},
	}
	s.appendTimeSeries(timeSeries)

	q := querier.NewQuerier[*task.Generic[*storage.PerGoroutineShard],
		*shard.DataStorage,
		*shard.LSS,
		*storage.PerGoroutineShard,
		*storage.HeadOnDisk,
	](s.head, querier.NewNoOpShardedDeduplicator, 0, 2, nil, nil)
	defer func() { _ = q.Close() }()
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "unknown_metric")

	// Act
	seriesSet := q.Select(s.context, false, nil, matcher)

	// Assert
	s.Equal([]storagetest.TimeSeries(nil), storagetest.TimeSeriesFromSeriesSet(seriesSet))
}

func (s *QuerierSuite) TestRangeQueryWithDataStorageLoading() {
	// Arrange
	timeSeries := []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 0},
				{Timestamp: 1, Value: 1},
				{Timestamp: 2, Value: 2},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test2"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 10},
				{Timestamp: 1, Value: 11},
				{Timestamp: 2, Value: 12},
			},
		},
	}
	s.appendTimeSeries(timeSeries)

	timeSeriesAfterUnload := []storagetest.TimeSeries{
		{
			Labels: timeSeries[0].Labels,
			Samples: []cppbridge.Sample{
				{Timestamp: 3, Value: 3},
			},
		},
		{
			Labels: timeSeries[1].Labels,
			Samples: []cppbridge.Sample{
				{Timestamp: 3, Value: 13},
			},
		},
	}

	q := querier.NewQuerier[*task.Generic[*storage.PerGoroutineShard],
		*shard.DataStorage,
		*shard.LSS,
		*storage.PerGoroutineShard,
		*storage.HeadOnDisk,
	](s.head, querier.NewNoOpShardedDeduplicator, 0, 3, nil, nil)
	defer func() { _ = q.Close() }()
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")

	// Act
	s.Require().NoError(services.UnloadUnusedSeriesDataWithHead(s.head))
	s.appendTimeSeries(timeSeriesAfterUnload)
	seriesSet := q.Select(s.context, false, nil, matcher)

	// Assert
	timeSeries[0].AppendSamples(timeSeriesAfterUnload[0].Samples...)
	timeSeries[1].AppendSamples(timeSeriesAfterUnload[1].Samples...)
	s.Equal(timeSeries, storagetest.TimeSeriesFromSeriesSet(seriesSet))
}

func (s *QuerierSuite) TestInstantQuery() {
	// Arrange
	timeSeries := []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test2"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 10},
			},
		},
	}
	s.appendTimeSeries(timeSeries)

	q := querier.NewQuerier[*task.Generic[*storage.PerGoroutineShard],
		*shard.DataStorage,
		*shard.LSS,
		*storage.PerGoroutineShard,
		*storage.HeadOnDisk,
	](s.head, querier.NewNoOpShardedDeduplicator, 0, 0, nil, nil)
	defer func() { _ = q.Close() }()
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")

	// Act
	seriesSet := q.Select(s.context, false, nil, matcher)

	// Assert
	s.Equal(timeSeries, storagetest.TimeSeriesFromSeriesSet(seriesSet))
}

func (s *QuerierSuite) TestInstantQueryWithDataStorageLoading() {
	// Arrange
	timeSeries := []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 0},
				{Timestamp: 1, Value: 1},
				{Timestamp: 2, Value: 2},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test2"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 10},
				{Timestamp: 1, Value: 11},
				{Timestamp: 2, Value: 12},
			},
		},
	}
	s.appendTimeSeries(timeSeries)

	timeSeriesAfterUnload := []storagetest.TimeSeries{
		{
			Labels: timeSeries[0].Labels,
			Samples: []cppbridge.Sample{
				{Timestamp: 3, Value: 3},
			},
		},
		{
			Labels: timeSeries[1].Labels,
			Samples: []cppbridge.Sample{
				{Timestamp: 3, Value: 13},
			},
		},
	}

	q := querier.NewQuerier[*task.Generic[*storage.PerGoroutineShard],
		*shard.DataStorage,
		*shard.LSS,
		*storage.PerGoroutineShard,
		*storage.HeadOnDisk,
	](s.head, querier.NewNoOpShardedDeduplicator, 0, 0, nil, nil)
	defer func() { _ = q.Close() }()
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")

	// Act
	s.Require().NoError(services.UnloadUnusedSeriesDataWithHead(s.head))
	s.appendTimeSeries(timeSeriesAfterUnload)
	seriesSet := q.Select(s.context, false, nil, matcher)

	// Assert
	s.Equal([]storagetest.TimeSeries{
		{
			Labels: timeSeries[0].Labels,
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 0},
			},
		},
		{
			Labels: timeSeries[1].Labels,
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 10},
			},
		},
	}, storagetest.TimeSeriesFromSeriesSet(seriesSet))
}
