package querier_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

type ChunkQuerier = querier.ChunkQuerier[
	*task.Generic[*shard.PerGoroutineShard],
	*shard.DataStorage,
	*shard.LSS,
	*shard.PerGoroutineShard,
	*storage.Head,
]

type ChunkQuerierSuite struct {
	suite.Suite
	dataDir string
	context context.Context
	head    *storage.Head
}

func TestChunkQuerierSuite(t *testing.T) {
	suite.Run(t, new(ChunkQuerierSuite))
}

func (s *ChunkQuerierSuite) SetupTest() {
	s.dataDir = s.createDataDirectory()
	s.context = context.Background()
	s.head = s.mustCreateHead(1)
}

func (s *ChunkQuerierSuite) TearDownTest() {
	runtime.GC()
}

func (s *ChunkQuerierSuite) createDataDirectory() string {
	dataDir := filepath.Join(s.T().TempDir(), "data")
	s.Require().NoError(os.MkdirAll(dataDir, os.ModeDir))
	return dataDir
}

func (s *ChunkQuerierSuite) mustCreateCatalog() *catalog.Catalog {
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

func (s *ChunkQuerierSuite) mustCreateHead(unloadDataStorageInterval time.Duration) *storage.Head {
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

func (s *ChunkQuerierSuite) appendTimeSeries(timeSeries []storagetest.TimeSeries) {
	storagetest.MustAppendTimeSeries(&s.Suite, s.head, timeSeries)
}

func (s *ChunkQuerierSuite) createQuerier(selectHints *prom_storage.SelectHints) *ChunkQuerier {
	q := querier.NewChunkQuerier[*task.Generic[*shard.PerGoroutineShard],
		*shard.DataStorage,
		*shard.LSS,
		*shard.PerGoroutineShard,
		*storage.Head,
	](s.head, querier.NewNoOpShardedDeduplicator, selectHints.Start, selectHints.End, nil)
	runtime.SetFinalizer(q, (*ChunkQuerier).Close)
	return q
}

func (s *ChunkQuerierSuite) timeSeriesFromChunkSeriesSet(
	chunkSeriesSet prom_storage.ChunkSeriesSet,
) []storagetest.TimeSeries {
	var result []storagetest.TimeSeries

	var chunkIterator chunks.Iterator
	var sampleIterator chunkenc.Iterator
	for chunkSeriesSet.Next() {
		chunkSeries := chunkSeriesSet.At()

		var samples []cppbridge.Sample
		chunkIterator = chunkSeries.Iterator(chunkIterator)
		for chunkIterator.Next() {
			meta := chunkIterator.At()
			sampleIterator = meta.Chunk.Iterator(sampleIterator)
			for sampleIterator.Next() != chunkenc.ValNone {
				ts, v := sampleIterator.At()
				samples = append(samples, cppbridge.Sample{Timestamp: ts, Value: v})
			}
		}

		result = append(result, storagetest.TimeSeries{Labels: chunkSeries.Labels(), Samples: samples})
	}

	return result
}

func (s *ChunkQuerierSuite) TestSelect() {
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

	selectHints := &prom_storage.SelectHints{Start: 0, End: 2, Step: 1}
	q := s.createQuerier(selectHints)
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")

	// Act
	chunkSeriesSet := q.Select(s.context, false, selectHints, matcher)

	// Assert
	s.Equal(timeSeries, s.timeSeriesFromChunkSeriesSet(chunkSeriesSet))
}

func (s *ChunkQuerierSuite) TestSelectWithoutData() {
	// Arrange
	s.appendTimeSeries([]storagetest.TimeSeries{
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
	})

	selectHints := &prom_storage.SelectHints{Start: 11, End: 22, Step: 1}
	q := s.createQuerier(selectHints)
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")

	// Act
	chunkSeriesSet := q.Select(s.context, false, selectHints, matcher)

	// Assert
	s.Equal([]storagetest.TimeSeries(nil), s.timeSeriesFromChunkSeriesSet(chunkSeriesSet))
}

func (s *ChunkQuerierSuite) TestSelectWithoutMatching() {
	// Arrange
	s.appendTimeSeries([]storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
			},
		},
	})

	selectHints := &prom_storage.SelectHints{Start: 0, End: 2, Step: 1}
	q := s.createQuerier(selectHints)
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "unknown_metric")

	// Act
	chunkSeriesSet := q.Select(s.context, false, selectHints, matcher)

	// Assert
	s.Equal([]storagetest.TimeSeries(nil), s.timeSeriesFromChunkSeriesSet(chunkSeriesSet))
}

func (s *ChunkQuerierSuite) TestSelectWithDataStorageLoading() {
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

	selectHints := &prom_storage.SelectHints{Start: 0, End: 3, Step: 1}
	q := s.createQuerier(selectHints)
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")

	s.Require().NoError(services.UnloadUnusedSeriesDataWithHead(s.head))

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
	s.appendTimeSeries(timeSeriesAfterUnload)

	// Act
	chunkSeriesSet := q.Select(s.context, false, selectHints, matcher)

	// Assert
	timeSeries[0].AppendSamples(timeSeriesAfterUnload[0].Samples...)
	timeSeries[1].AppendSamples(timeSeriesAfterUnload[1].Samples...)
	s.Equal(timeSeries, s.timeSeriesFromChunkSeriesSet(chunkSeriesSet))
}

func (s *ChunkQuerierSuite) TestSelectOnClosedHead() {
	// Arrange
	s.appendTimeSeries([]storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
			},
		},
	})

	selectHints := &prom_storage.SelectHints{Start: 0, End: 2, Step: 1}
	q := s.createQuerier(selectHints)
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")

	s.Require().NoError(s.head.Close())

	// Act
	chunkSeriesSet := q.Select(s.context, false, selectHints, matcher)

	// Assert
	s.Equal([]storagetest.TimeSeries(nil), s.timeSeriesFromChunkSeriesSet(chunkSeriesSet))
}

func (s *ChunkQuerierSuite) TestLabelNames() {
	// Arrange
	s.appendTimeSeries([]storagetest.TimeSeries{
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
	})

	q := s.createQuerier(&prom_storage.SelectHints{})
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric0")

	// Act
	names, anns, err := q.LabelNames(s.context, &prom_storage.LabelHints{Limit: 10}, matcher)

	// Assert
	s.Require().NoError(err)
	s.Equal([]string{"__name__", "job0"}, names)
	s.Len(anns.AsErrors(), 1)
}

func (s *ChunkQuerierSuite) TestLabelNamesWithLimit() {
	// Arrange
	s.appendTimeSeries([]storagetest.TimeSeries{
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
	})

	q := s.createQuerier(&prom_storage.SelectHints{})
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric0")

	// Act
	names, anns, err := q.LabelNames(s.context, &prom_storage.LabelHints{Limit: 1}, matcher)

	// Assert
	s.Require().NoError(err)
	s.Equal([]string{"__name__"}, names)
	s.Len(anns.AsErrors(), 1)
}

func (s *ChunkQuerierSuite) TestLabelNamesNoMatches() {
	// Arrange
	s.appendTimeSeries([]storagetest.TimeSeries{
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
	})

	q := s.createQuerier(&prom_storage.SelectHints{})
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric3")

	// Act
	names, anns, err := q.LabelNames(s.context, &prom_storage.LabelHints{Limit: 1}, matcher)

	// Assert
	s.Require().NoError(err)
	s.Equal([]string{}, names)
	s.Len(anns.AsErrors(), 1)
}

func (s *ChunkQuerierSuite) TestLabelValues() {
	// Arrange
	s.appendTimeSeries([]storagetest.TimeSeries{
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
	})

	q := s.createQuerier(&prom_storage.SelectHints{})
	matcher, _ := labels.NewMatcher(labels.MatchRegexp, "__name__", "metric.*")

	// Act
	names, anns, err := q.LabelValues(s.context, "__name__", &prom_storage.LabelHints{Limit: 10}, matcher)

	// Assert
	s.Require().NoError(err)
	s.Equal([]string{"metric0", "metric1"}, names)
	s.Len(anns.AsErrors(), 1)
}

func (s *ChunkQuerierSuite) TestLabelValuesNoMatches() {
	// Arrange
	s.appendTimeSeries([]storagetest.TimeSeries{
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
	})

	q := s.createQuerier(&prom_storage.SelectHints{})
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric2")

	// Act
	names, anns, err := q.LabelValues(s.context, "__name__", &prom_storage.LabelHints{Limit: 10}, matcher)

	// Assert
	s.Require().NoError(err)
	s.Equal([]string{}, names)
	s.Len(anns.AsErrors(), 1)
}

func (s *ChunkQuerierSuite) TestLabelValuesNoMatchesOnName() {
	// Arrange
	s.appendTimeSeries([]storagetest.TimeSeries{
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
	})

	q := s.createQuerier(&prom_storage.SelectHints{})
	matcher, _ := labels.NewMatcher(labels.MatchRegexp, "__name__", "metric.*")

	// Act
	names, anns, err := q.LabelValues(s.context, "instance", &prom_storage.LabelHints{Limit: 10}, matcher)

	// Assert
	s.Require().NoError(err)
	s.Equal([]string{}, names)
	s.Len(anns.AsErrors(), 1)
}
