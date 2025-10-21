package querier_test

import (
	"context"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"testing"
	"time"
)

type SeriesSetTestSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc

	timeSeries []storagetest.TimeSeries
	lss        *shard.LSS
	ds         *shard.DataStorage
}

func TestSeriesSetTestSuite(t *testing.T) {
	suite.Run(t, new(SeriesSetTestSuite))
}

func (s *SeriesSetTestSuite) SetupTest() {
	if s.cancel != nil {
		s.cancel()
	}
	s.ctx, s.cancel = context.WithTimeout(context.Background(), time.Minute)

	s.lss = shard.NewLSS()
	s.ds = shard.NewDataStorage()

	s.timeSeries = []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 10, Value: 0},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 11, Value: 1},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 12, Value: 3},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 13, Value: 5},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test2"),
			Samples: []cppbridge.Sample{
				{Timestamp: 11, Value: 1},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test2"),
			Samples: []cppbridge.Sample{
				{Timestamp: 12, Value: 2},
			},
		},
	}
}

func (s *SeriesSetTestSuite) query(start, end int64, matchers ...model.LabelMatcher) *querier.SeriesSet {
	selector, snapshot, err := s.lss.QuerySelector(0, matchers)
	require.NoError(s.T(), err)
	if selector == 0 || snapshot == nil {
		return &querier.SeriesSet{}
	}

	lssQueryResult := snapshot.Query(selector)
	if lssQueryResult.Status() == cppbridge.LSSQueryStatusNoMatch {
		return &querier.SeriesSet{}
	}

	dsQueryResult := s.ds.Query(cppbridge.HeadDataStorageQuery{
		StartTimestampMs: start,
		EndTimestampMs:   end,
		LabelSetIDs:      lssQueryResult.IDs(),
	})

	require.Equal(s.T(), cppbridge.DataStorageQueryStatusSuccess, dsQueryResult.Status)
	return querier.NewSeriesSet(start, end, lssQueryResult, snapshot, dsQueryResult.SerializedData)
}

func (s *SeriesSetTestSuite) assertEqual(timeSeries []storagetest.TimeSeries, seriesSet *querier.SeriesSet) {
	for seriesSet.Next() {
		series := seriesSet.At()
		labelSet := series.Labels()
		iterator := series.Iterator(nil)
		for iterator.Next() == chunkenc.ValFloat {
			ts, v := iterator.At()
			found := false
			for i := range timeSeries {
				if timeSeries[i].Labels.String() == labelSet.String() && timeSeries[i].Samples[0].Timestamp == ts && timeSeries[i].Samples[0].Value == v {
					timeSeries = append(timeSeries[:i], timeSeries[i+1:]...)
					found = true
					break
				}
			}
			require.True(s.T(), found)
		}
	}

	require.Empty(s.T(), timeSeries)
}

func (s *SeriesSetTestSuite) TestQueryAllValues() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	var start int64 = 0
	var end int64 = 50

	expected := s.timeSeries

	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, s.timeSeries...)

	// Act
	seriesSet := s.query(start, end, matcher)

	// Assert
	s.assertEqual(expected, seriesSet)
}

func (s *SeriesSetTestSuite) TestQueryNoValues() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	var start int64 = 0
	var end int64 = 1

	expected := []storagetest.TimeSeries{}
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, s.timeSeries...)

	// Act
	seriesSet := s.query(start, end, matcher)

	// Assert
	s.assertEqual(expected, seriesSet)
}

func (s *SeriesSetTestSuite) TestQuerySingleSeries() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "job",
		Value:       "test",
		MatcherType: model.MatcherTypeExactMatch,
	}

	var start int64 = 0
	var end int64 = 50

	expected := s.timeSeries[:4]
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, s.timeSeries...)

	// Act
	seriesSet := s.query(start, end, matcher)

	// Assert
	s.assertEqual(expected, seriesSet)
}

func (s *SeriesSetTestSuite) TestQuerySingleSample() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "job",
		Value:       "test",
		MatcherType: model.MatcherTypeExactMatch,
	}

	var start int64 = 13
	var end int64 = 13

	expected := s.timeSeries[3:4]
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, s.timeSeries...)

	// Act
	seriesSet := s.query(start, end, matcher)

	// Assert
	s.assertEqual(expected, seriesSet)
}

func (s *SeriesSetTestSuite) TestQueryCutByUpperLimit() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	var start int64 = 10
	var end int64 = 11

	expected := []storagetest.TimeSeries{s.timeSeries[0], s.timeSeries[1], s.timeSeries[4]}
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, s.timeSeries...)

	// Act
	seriesSet := s.query(start, end, matcher)

	// Assert
	s.assertEqual(expected, seriesSet)
}

func (s *SeriesSetTestSuite) TestQueryCutByLowerLimit() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	var start int64 = 11
	var end int64 = 50

	expected := s.timeSeries[1:]
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, s.timeSeries...)

	// Act
	seriesSet := s.query(start, end, matcher)

	// Assert
	s.assertEqual(expected, seriesSet)
}

func (s *SeriesSetTestSuite) TestQueryLargeChunks() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	var start int64 = 0
	var end int64 = 1000

	var timeSeries []storagetest.TimeSeries
	for i := 0; i < 500; i++ {
		timeSeries = append(timeSeries, storagetest.TimeSeries{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: int64(i), Value: float64(i)},
			},
		})
	}
	expected := timeSeries
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, timeSeries...)

	// Act
	seriesSet := s.query(start, end, matcher)

	// Assert
	s.assertEqual(expected, seriesSet)
}

func (s *SeriesSetTestSuite) TestQueryEmptyStorage() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	var start int64 = 0
	var end int64 = 1000

	expected := []storagetest.TimeSeries{}
	// Act
	seriesSet := s.query(start, end, matcher)

	// Assert
	s.assertEqual(expected, seriesSet)
}
