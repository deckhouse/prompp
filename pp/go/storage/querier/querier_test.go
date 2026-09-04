package querier_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	prom_storage "github.com/prometheus/prometheus/storage"
)

const (
	numberOfShards uint16 = 2

	maxSegmentSize uint32 = 1024
)

type Querier = querier.Querier[
	*task.Generic[*shard.PerGoroutineShard],
	*shard.DataStorage,
	*shard.LSS,
	*shard.PerGoroutineShard,
	*storage.Head,
]

type QuerierSuite struct {
	suite.Suite
	dataDir string
	context context.Context
	head    *storage.Head

	hints          *prom_storage.SelectHints
	scrapeInterval int64
	retentionMS    int64
	downsamplingMS int64
}

func TestQuerierSuite(t *testing.T) {
	suite.Run(t, new(QuerierSuite))
}

func (s *QuerierSuite) SetupTest() {
	s.dataDir = s.createDataDirectory()
	s.context = context.Background()
	s.head = s.mustCreateHead(1)
	s.hints = &prom_storage.SelectHints{
		Start: 0,
		End:   200,
		Range: 100,
	}
	s.scrapeInterval = 1
	s.retentionMS = 10000
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

func (s *QuerierSuite) mustCreateHead(unloadDataStorageInterval time.Duration) *storage.Head {
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
	storagetest.MustAppendTimeSeries(s.T().Context(), s.Require().NoError, s.head, timeSeries)
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

	q := querier.NewQuerier[*task.Generic[*shard.PerGoroutineShard],
		*shard.DataStorage,
		*shard.LSS,
		*shard.PerGoroutineShard,
		*storage.Head,
	](s.head, querier.NewNoOpShardedDeduplicator, 0, 2, s.scrapeInterval, 0, s.retentionMS, s.downsamplingMS, nil)
	defer func() { _ = q.Close() }()
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")

	// Act
	seriesSet := q.Select(s.context, false, &prom_storage.SelectHints{Start: 0, End: 2, Step: 1}, matcher)

	// Assert
	s.Equal(timeSeries, storagetest.TimeSeriesFromSeriesSet(seriesSet, true))
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

	q := querier.NewQuerier[*task.Generic[*shard.PerGoroutineShard],
		*shard.DataStorage,
		*shard.LSS,
		*shard.PerGoroutineShard,
		*storage.Head,
	](s.head, querier.NewNoOpShardedDeduplicator, 0, 2, s.scrapeInterval, 0, s.retentionMS, s.downsamplingMS, nil)
	defer func() { _ = q.Close() }()
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "unknown_metric")

	// Act
	seriesSet := q.Select(s.context, false, &prom_storage.SelectHints{Start: 0, End: 2, Step: 1}, matcher)

	// Assert
	s.Equal([]storagetest.TimeSeries{}, storagetest.TimeSeriesFromSeriesSet(seriesSet, true))
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

	q := querier.NewQuerier[*task.Generic[*shard.PerGoroutineShard],
		*shard.DataStorage,
		*shard.LSS,
		*shard.PerGoroutineShard,
		*storage.Head,
	](s.head, querier.NewNoOpShardedDeduplicator, 0, 3, s.scrapeInterval, 0, s.retentionMS, s.downsamplingMS, nil)
	defer func() { _ = q.Close() }()
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")

	// Act
	s.Require().NoError(services.UnloadUnusedSeriesDataWithHead(s.head))
	s.appendTimeSeries(timeSeriesAfterUnload)
	seriesSet := q.Select(s.context, false, &prom_storage.SelectHints{Start: 0, End: 3, Step: 1}, matcher)

	// Assert
	timeSeries[0].AppendSamples(timeSeriesAfterUnload[0].Samples...)
	timeSeries[1].AppendSamples(timeSeriesAfterUnload[1].Samples...)
	s.Equal(timeSeries, storagetest.TimeSeriesFromSeriesSet(seriesSet, true))
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

	q := querier.NewQuerier[*task.Generic[*shard.PerGoroutineShard],
		*shard.DataStorage,
		*shard.LSS,
		*shard.PerGoroutineShard,
		*storage.Head,
	](s.head, querier.NewNoOpShardedDeduplicator, 0, 0, s.scrapeInterval, 0, s.retentionMS, s.downsamplingMS, nil)
	defer func() { _ = q.Close() }()
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")

	// Act
	seriesSet := q.Select(s.context, false, &prom_storage.SelectHints{}, matcher)

	// Assert
	s.Equal(timeSeries, storagetest.TimeSeriesFromSeriesSet(seriesSet, true))
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

	q := querier.NewQuerier[*task.Generic[*shard.PerGoroutineShard],
		*shard.DataStorage,
		*shard.LSS,
		*shard.PerGoroutineShard,
		*storage.Head,
	](s.head, querier.NewNoOpShardedDeduplicator, 0, 0, s.scrapeInterval, 0, s.retentionMS, s.downsamplingMS, nil)
	defer func() { _ = q.Close() }()
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")

	// Act
	s.Require().NoError(services.UnloadUnusedSeriesDataWithHead(s.head))
	s.appendTimeSeries(timeSeriesAfterUnload)
	seriesSet := q.Select(s.context, false, &prom_storage.SelectHints{}, matcher)

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
	}, storagetest.TimeSeriesFromSeriesSet(seriesSet, true))
}

func (s *QuerierSuite) TestLabelNames() {
	// Arrange
	timeSeries := []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric0", "job0", "test0"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric1", "job1", "test1"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 10},
			},
		},
	}
	s.appendTimeSeries(timeSeries)

	q := querier.NewQuerier(
		s.head, querier.NewNoOpShardedDeduplicator, 0, 2, s.scrapeInterval, 0, s.retentionMS, s.downsamplingMS, nil,
	)
	defer func() { _ = q.Close() }()
	matcher, err := labels.NewMatcher(labels.MatchEqual, "__name__", "metric0")
	s.Require().NoError(err)
	hints := &prom_storage.LabelHints{Limit: 10}

	// Act
	names, anns, err := q.LabelNames(s.context, hints, matcher)
	s.Require().NoError(err)

	// Assert
	s.Equal([]string{"__name__", "job0"}, names)
	s.Len(anns.AsErrors(), 1)
}

func (s *QuerierSuite) TestLabelNamesWithLimit() {
	// Arrange
	timeSeries := []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric0", "job0", "test0"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric1", "job1", "test1"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 10},
			},
		},
	}
	s.appendTimeSeries(timeSeries)

	q := querier.NewQuerier(
		s.head, querier.NewNoOpShardedDeduplicator, 0, 2, s.scrapeInterval, 0, s.retentionMS, s.downsamplingMS, nil,
	)
	defer func() { _ = q.Close() }()
	matcher, err := labels.NewMatcher(labels.MatchEqual, "__name__", "metric0")
	s.Require().NoError(err)
	hints := &prom_storage.LabelHints{Limit: 1}

	// Act
	names, anns, err := q.LabelNames(s.context, hints, matcher)
	s.Require().NoError(err)

	// Assert
	s.Equal([]string{"__name__"}, names)
	s.Len(anns.AsErrors(), 1)
}

func (s *QuerierSuite) TestLabelNamesNoMatches() {
	// Arrange
	timeSeries := []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric0", "job0", "test0"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric1", "job1", "test1"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 10},
			},
		},
	}
	s.appendTimeSeries(timeSeries)

	q := querier.NewQuerier(
		s.head, querier.NewNoOpShardedDeduplicator, 0, 2, s.scrapeInterval, 0, s.retentionMS, s.downsamplingMS, nil,
	)
	defer func() { _ = q.Close() }()
	matcher, err := labels.NewMatcher(labels.MatchEqual, "__name__", "metric3")
	s.Require().NoError(err)
	hints := &prom_storage.LabelHints{Limit: 1}

	// Act
	names, anns, err := q.LabelNames(s.context, hints, matcher)
	s.Require().NoError(err)

	// Assert
	s.Equal([]string{}, names)
	s.Len(anns.AsErrors(), 1)
}

func (s *QuerierSuite) TestLabelValues() {
	// Arrange
	timeSeries := []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric0", "job0", "test0"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric1", "job1", "test1"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 10},
			},
		},
	}
	s.appendTimeSeries(timeSeries)

	q := querier.NewQuerier(
		s.head, querier.NewNoOpShardedDeduplicator, 0, 2, s.scrapeInterval, 0, s.retentionMS, s.downsamplingMS, nil,
	)
	defer func() { _ = q.Close() }()
	matcher, err := labels.NewMatcher(labels.MatchRegexp, "__name__", "metric.*")
	s.Require().NoError(err)
	hints := &prom_storage.LabelHints{Limit: 10}

	// Act
	names, anns, err := q.LabelValues(s.context, "__name__", hints, matcher)
	s.Require().NoError(err)

	// Assert
	s.Equal([]string{"metric0", "metric1"}, names)
	s.Len(anns.AsErrors(), 1)
}

func (s *QuerierSuite) TestLabelValuesNoMatches() {
	// Arrange
	timeSeries := []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric0", "job0", "test0"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric1", "job1", "test1"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 10},
			},
		},
	}
	s.appendTimeSeries(timeSeries)

	q := querier.NewQuerier(
		s.head, querier.NewNoOpShardedDeduplicator, 0, 2, s.scrapeInterval, 0, s.retentionMS, s.downsamplingMS, nil,
	)
	defer func() { _ = q.Close() }()
	matcher, err := labels.NewMatcher(labels.MatchEqual, "__name__", "metric2")
	s.Require().NoError(err)
	hints := &prom_storage.LabelHints{Limit: 10}

	// Act
	names, anns, err := q.LabelValues(s.context, "__name__", hints, matcher)
	s.Require().NoError(err)

	// Assert
	s.Equal([]string{}, names)
	s.Len(anns.AsErrors(), 1)
}

func (s *QuerierSuite) TestLabelValuesNoMatchesOnName() {
	// Arrange
	timeSeries := []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric0", "job0", "test0"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric1", "job1", "test1"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 10},
			},
		},
	}
	s.appendTimeSeries(timeSeries)

	q := querier.NewQuerier(
		s.head, querier.NewNoOpShardedDeduplicator, 0, 2, s.scrapeInterval, 0, s.retentionMS, s.downsamplingMS, nil,
	)
	defer func() { _ = q.Close() }()
	matcher, err := labels.NewMatcher(labels.MatchRegexp, "__name__", "metric.*")
	s.Require().NoError(err)
	hints := &prom_storage.LabelHints{Limit: 10}

	// Act
	names, anns, err := q.LabelValues(s.context, "instance", hints, matcher)
	s.Require().NoError(err)

	// Assert
	s.Equal([]string{}, names)
	s.Len(anns.AsErrors(), 1)
}

func (s *QuerierSuite) TestWouldDownsampleFalseWhenRangeLessThanRetention() {
	// Arrange: range (maxt-mint) less than retention, so no downsampling should apply
	q := querier.NewQuerier(
		s.head,
		querier.NewNoOpShardedDeduplicator,
		0,     // mint
		5000,  // maxt (range = 5000, less than 10000 retention)
		1,     // scrapeInterval
		0,     // headMinTSMS
		10000, // retentionMS
		60000, // downsamplingMS (not applied)
		nil,
	)
	defer func() { _ = q.Close() }()

	// Act
	wouldDownsample := q.WouldDownsample()

	// Assert
	s.False(wouldDownsample)
}

func (s *QuerierSuite) TestWouldDownsampleTrueWhenRangeGreaterThanRetention() {
	// Arrange: range (maxt-mint) greater than retention, so downsampling should apply
	q := querier.NewQuerier(
		s.head,
		querier.NewNoOpShardedDeduplicator,
		0,     // mint
		15000, // maxt (range = 15000, greater than 10000 retention)
		1,     // scrapeInterval
		0,     // headMinTSMS
		10000, // retentionMS
		60000, // downsamplingMS
		nil,
	)
	defer func() { _ = q.Close() }()

	// Act
	wouldDownsample := q.WouldDownsample()

	// Assert
	s.True(wouldDownsample)
}

func (s *QuerierSuite) TestWouldDownsampleFalseWhenDownsamplingDisabled() {
	// Arrange: downsampling disabled (NoDownsampling)
	q := querier.NewQuerier(
		s.head,
		querier.NewNoOpShardedDeduplicator,
		0,                        // mint
		15000,                    // maxt (range = 15000, greater than 10000 retention)
		1,                        // scrapeInterval
		0,                        // headMinTSMS
		10000,                    // retentionMS
		cppbridge.NoDownsampling, // downsamplingMS disabled
		nil,
	)
	defer func() { _ = q.Close() }()

	// Act
	wouldDownsample := q.WouldDownsample()

	// Assert
	s.False(wouldDownsample)
}

func (s *QuerierSuite) TestSelectRangeBypassesDownsamplingForRateFunction() {
	// Arrange: setup with downsampling enabled, query range > retention
	timeSeries := []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "http_requests_total", "job", "api"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 100},
				{Timestamp: 1000, Value: 200},
				{Timestamp: 2000, Value: 300},
				{Timestamp: 10000, Value: 400}, // Gap: 8s
				{Timestamp: 11000, Value: 500},
				{Timestamp: 12000, Value: 600},
			},
		},
	}
	s.appendTimeSeries(timeSeries)

	// Retention: 5s, Downsampling: 2s, Query range: 12s (> retention)
	downsamplingMS := int64(2000)
	q := querier.NewQuerier(
		s.head,
		querier.NewNoOpShardedDeduplicator,
		0,     // mint
		12000, // maxt (range = 12s > 5s retention)
		1000,  // scrapeInterval (1s)
		0,     // headMinTSMS
		5000,  // retentionMS
		downsamplingMS,
		nil,
	)
	defer func() { _ = q.Close() }()

	matcher := labels.MustNewMatcher(labels.MatchEqual, "__name__", "http_requests_total")

	// Act: Select with rate() function (in allow-list)
	hints := &prom_storage.SelectHints{
		Func:  "rate",
		Range: 1000,
		Start: 0,
		End:   12000,
	}
	ss := q.Select(s.context, false, hints, matcher)

	// Assert: Should get series with raw data (not downsampled)
	result := storagetest.TimeSeriesFromSeriesSet(ss, true)
	s.Len(result, 1)
	s.Equal(timeSeries[0].Labels, result[0].Labels)
	// Should have all 6 raw samples (downsampling was bypassed for rate())
	s.Len(result[0].Samples, len(timeSeries[0].Samples))
}

func (s *QuerierSuite) TestSelectRangeDoesNotBypassDownsamplingForNonRateFunction() {
	// Arrange: setup with downsampling enabled, query range > retention
	timeSeries := []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "http_requests_total", "job", "api"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 100},
				{Timestamp: 1000, Value: 200},
				{Timestamp: 2000, Value: 300},
				{Timestamp: 10000, Value: 400},
				{Timestamp: 11000, Value: 500},
				{Timestamp: 12000, Value: 600},
			},
		},
	}
	s.appendTimeSeries(timeSeries)

	// Retention: 5s, Downsampling: 2s, Query range: 12s (> retention)
	downsamplingMS := int64(2000)
	q := querier.NewQuerier(
		s.head,
		querier.NewNoOpShardedDeduplicator,
		0,     // mint
		12000, // maxt
		1000,  // scrapeInterval
		0,     // headMinTSMS
		5000,  // retentionMS
		downsamplingMS,
		nil,
	)
	defer func() { _ = q.Close() }()

	matcher := labels.MustNewMatcher(labels.MatchEqual, "__name__", "http_requests_total")

	// Act: Select with sum() function (NOT in allow-list)
	hints := &prom_storage.SelectHints{
		Func:  "sum",
		Range: 1000,
		Start: 0,
		End:   12000,
	}
	ss := q.Select(s.context, false, hints, matcher)

	// Assert: Downsampling was NOT bypassed for sum()
	result := storagetest.TimeSeriesFromSeriesSet(ss, true)
	s.Len(result, 1)
	s.Equal(timeSeries[0].Labels, result[0].Labels)
	// For cross-series/aggregation functions, downsampling applies normally
	// The result should be processed but not error out
	s.NoError(ss.Err())
}

func (s *QuerierSuite) TestSelectRangeNoBypassWhenDownsamplingDisabled() {
	// Arrange: downsampling disabled
	timeSeries := []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "http_requests_total", "job", "api"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 100},
				{Timestamp: 1000, Value: 200},
				{Timestamp: 2000, Value: 300},
				{Timestamp: 10000, Value: 400},
				{Timestamp: 11000, Value: 500},
				{Timestamp: 12000, Value: 600},
			},
		},
	}
	s.appendTimeSeries(timeSeries)

	// Downsampling: NoDownsampling (disabled)
	q := querier.NewQuerier(
		s.head,
		querier.NewNoOpShardedDeduplicator,
		0,                        // mint
		12000,                    // maxt
		1000,                     // scrapeInterval
		0,                        // headMinTSMS
		5000,                     // retentionMS
		cppbridge.NoDownsampling, // disabled
		nil,
	)
	defer func() { _ = q.Close() }()

	matcher := labels.MustNewMatcher(labels.MatchEqual, "__name__", "http_requests_total")

	// Act: Select with rate() function (would trigger bypass if enabled)
	hints := &prom_storage.SelectHints{
		Func:  "rate",
		Range: 1000,
		Start: 0,
		End:   12000,
	}
	ss := q.Select(s.context, false, hints, matcher)

	// Assert: Should get raw data (no downsampling to bypass)
	result := storagetest.TimeSeriesFromSeriesSet(ss, true)
	s.Len(result, 1)
	s.Equal(timeSeries[0].Labels, result[0].Labels)
	s.Len(result[0].Samples, len(timeSeries[0].Samples))
}

func (s *QuerierSuite) TestSelectRangeBypassDownsamplingForIncreaseFunction() {
	// Arrange: test with increase() function (also in allow-list)
	timeSeries := []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "errors_total", "service", "auth"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 10},
				{Timestamp: 1000, Value: 20},
				{Timestamp: 8000, Value: 30}, // Gap: 7s
				{Timestamp: 9000, Value: 40},
			},
		},
	}
	s.appendTimeSeries(timeSeries)

	downsamplingMS := int64(3000)
	q := querier.NewQuerier(
		s.head,
		querier.NewNoOpShardedDeduplicator,
		0,     // mint
		10000, // maxt (range = 10s > 5s retention)
		1000,  // scrapeInterval
		0,     // headMinTSMS
		5000,  // retentionMS
		downsamplingMS,
		nil,
	)
	defer func() { _ = q.Close() }()

	matcher := labels.MustNewMatcher(labels.MatchEqual, "__name__", "errors_total")

	// Act: Select with increase() function
	hints := &prom_storage.SelectHints{
		Func:  "increase",
		Range: 1000,
		Start: 0,
		End:   10000,
	}
	ss := q.Select(s.context, false, hints, matcher)

	// Assert: Should bypass downsampling and get raw data
	result := storagetest.TimeSeriesFromSeriesSet(ss, true)
	s.Len(result, 1)
	s.Equal(timeSeries[0].Labels, result[0].Labels)
	// Should have all 4 raw samples (downsampling was bypassed for increase())
	s.Len(result[0].Samples, len(timeSeries[0].Samples))
}
