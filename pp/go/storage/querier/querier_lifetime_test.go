package querier_test

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	ppstorage "github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

func (s *QuerierSuite) TestCloseStopsInstantMergeBeforeFirstNext() {
	// Arrange
	s.appendLifetimeSeries()
	q := querier.NewQuerier(s.head, querier.NewNoOpShardedDeduplicator, 5, 5, s.scrapeInterval, 0, nil)
	s.T().Cleanup(func() { s.Require().NoError(q.Close()) })
	matcher := labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "lifetime_metric")
	started := q.Select(s.T().Context(), true, nil, matcher)
	unread := q.Select(s.T().Context(), true, nil, matcher)
	s.Require().True(started.Next())
	series := started.At()
	iterator := series.Iterator(nil)
	s.Require().Equal(chunkenc.ValFloat, iterator.Next())

	// Act
	err := q.Close()

	// Assert
	s.Require().NoError(err)
	s.False(unread.Next())
	s.False(started.Next())
	s.Equal("lifetime_metric", series.Labels().Get(labels.MetricName))
	s.Equal(int64(5), iterator.AtT())
}

func (s *QuerierSuite) TestCloseStopsRangeSelectionsAndBorrowedIterators() {
	// Arrange
	s.appendLifetimeSeries()
	s.Require().NoError(querier.SetSelectFuncOptimize("all"))
	s.T().Cleanup(querier.SetDefaultOptimizeType)
	matcher := labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "lifetime_metric")

	for _, tc := range []struct {
		name string
		fn   string
	}{
		{name: "range"},
		{name: "aggregation", fn: "min_over_time"},
		{name: "cross series", fn: "sum"},
	} {
		s.Run(tc.name, func() {
			// Arrange
			q := querier.NewQuerier(s.head, querier.NewNoOpShardedDeduplicator, 0, 8, s.scrapeInterval, 0, nil)
			s.T().Cleanup(func() { s.Require().NoError(q.Close()) })
			q.IsPossibleToOptimize = func([]*cppbridge.LSSQueryResult, *storage.SelectHints, int64, int64) bool {
				return true
			}
			hints := &storage.SelectHints{Start: 2, End: 8, Step: 2, Range: 3, LookbackDelta: 3, Func: tc.fn}
			started := q.Select(s.T().Context(), true, hints, matcher)
			unread := q.Select(s.T().Context(), true, hints, matcher)
			s.Require().NoError(unread.Err())
			s.Require().NoError(started.Err())
			s.Require().True(started.Next())
			series := started.At()
			if tc.fn == "sum" {
				s.Require().NotEmpty(series.Labels().Get("__head__shard_id"))
			}
			originalLabels := series.Labels().String()
			iterator := series.Iterator(nil)
			s.Require().Equal(chunkenc.ValFloat, iterator.Seek(1))
			_, value := iterator.At()
			s.Require().Equal(float64(7), value)

			// Act
			err := q.Close()
			repeatedErr := q.Close()

			// Assert
			s.Require().NoError(err)
			s.Require().NoError(repeatedErr)
			s.False(started.Next())
			s.False(unread.Next())
			s.Equal(originalLabels, series.Labels().String())
			s.Equal(chunkenc.ValNone, iterator.Next())
			s.Equal(chunkenc.ValNone, iterator.Seek(5))
			s.Equal(chunkenc.ValNone, series.Iterator(nil).Next())
			s.Equal(chunkenc.ValNone, series.Iterator(iterator).Seek(1))
		})
	}
}

func (s *QuerierSuite) TestRangeSelectDoesNotPublishResidentShardOnLoadFailure() {
	// Arrange
	s.appendLifetimeSeries()
	resident, unloaded := s.head.Shards()[0], s.head.Shards()[1]
	// Constant-valued chunks are not unloadable. Force the cold series into an unloadable encoding.
	unloaded.DataStorage().Encode(0, 9, 1.25)
	unloaded.DataStorage().Encode(0, 10, 2.75)
	s.Require().NoError(unloaded.UnloadUnusedSeriesData())
	snapshotFile := ppstorage.GetUnloadedDataStorageFilename(filepath.Join(s.dataDir, s.head.ID()), unloaded.ShardID())
	s.Require().NoError(os.Truncate(snapshotFile, 1))
	q := querier.NewQuerier(s.head, querier.NewNoOpShardedDeduplicator, 0, 10, s.scrapeInterval, 0, nil)
	s.T().Cleanup(func() { s.Require().NoError(q.Close()) })
	matcher := labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "lifetime_metric")
	residentOnly := labels.MustNewMatcher(labels.MatchEqual, "shard", strconv.Itoa(int(resident.ShardID())))

	// Act
	partial := q.Select(s.T().Context(), true, nil, matcher)
	complete := q.Select(s.T().Context(), true, nil, matcher, residentOnly)

	// Assert
	s.Require().NoError(partial.Err())
	s.False(partial.Next())
	s.Require().NoError(complete.Err())
	s.Require().True(complete.Next())
	it := complete.At().Iterator(nil)
	s.Require().Equal(chunkenc.ValFloat, it.Next())
	timestamp, value := it.At()
	s.Equal(int64(1), timestamp)
	s.Equal(float64(7), value)
	s.False(complete.Next())
}

// appendLifetimeSeries places a series on each shard regardless of the label hash.
func (s *QuerierSuite) appendLifetimeSeries() {
	s.T().Cleanup(func() { s.Require().NoError(s.head.Close()) })

	for _, sh := range s.head.Shards() {
		storagetest.MustAppendTimeSeriesToLSSAndDataStorage(sh.LSS(), sh.DataStorage(), storagetest.TimeSeries{
			Labels: labels.FromStrings(labels.MetricName, "lifetime_metric", "shard", strconv.Itoa(int(sh.ShardID()))),
			Samples: []cppbridge.Sample{
				{Timestamp: 1, Value: 7}, {Timestamp: 3, Value: 7}, {Timestamp: 5, Value: 7}, {Timestamp: 7, Value: 7},
			},
		})
	}
}
