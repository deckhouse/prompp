package querier_test

import (
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"testing"
)

type SeriesSetTestSuite struct {
	suite.Suite

	timeSeries []storagetest.TimeSeries
	lss        *shard.LSS
	ds         *shard.DataStorage
}

func TestSeriesSetTestSuite(t *testing.T) {
	suite.Run(t, new(SeriesSetTestSuite))
}

func (s *SeriesSetTestSuite) SetupTest() {
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

func query(t testing.TB, lss *shard.LSS, ds *shard.DataStorage, start, end int64, matchers ...model.LabelMatcher) *querier.SeriesSet {
	selector, snapshot, err := lss.QuerySelector(0, matchers)
	require.NoError(t, err)
	if selector == 0 || snapshot == nil {
		return &querier.SeriesSet{}
	}

	lssQueryResult := snapshot.Query(selector)
	if lssQueryResult.Status() == cppbridge.LSSQueryStatusNoMatch {
		return &querier.SeriesSet{}
	}

	dsQueryResult := ds.Query(cppbridge.HeadDataStorageQuery{
		StartTimestampMs: start,
		EndTimestampMs:   end,
		LabelSetIDs:      lssQueryResult.IDs(),
	})

	require.Equal(t, cppbridge.DataStorageQueryStatusSuccess, dsQueryResult.Status)
	return querier.NewSeriesSet(start, end, lssQueryResult, snapshot, dsQueryResult.SerializedData)
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
	seriesSet := query(s.T(), s.lss, s.ds, start, end, matcher)

	// Assert
	require.Equal(s.T(), expected, storagetest.TimeSeriesFromSeriesSet(seriesSet, false))
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
	seriesSet := query(s.T(), s.lss, s.ds, start, end, matcher)

	// Assert
	require.Equal(s.T(), expected, storagetest.TimeSeriesFromSeriesSet(seriesSet, false))
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
	seriesSet := query(s.T(), s.lss, s.ds, start, end, matcher)

	// Assert
	require.Equal(s.T(), expected, storagetest.TimeSeriesFromSeriesSet(seriesSet, false))
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
	seriesSet := query(s.T(), s.lss, s.ds, start, end, matcher)

	// Assert
	require.Equal(s.T(), expected, storagetest.TimeSeriesFromSeriesSet(seriesSet, false))
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
	seriesSet := query(s.T(), s.lss, s.ds, start, end, matcher)

	// Assert
	require.Equal(s.T(), expected, storagetest.TimeSeriesFromSeriesSet(seriesSet, false))
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
	seriesSet := query(s.T(), s.lss, s.ds, start, end, matcher)

	// Assert
	require.Equal(s.T(), expected, storagetest.TimeSeriesFromSeriesSet(seriesSet, false))
}

func (s *SeriesSetTestSuite) TestQueryLargeChunks() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	var start int64 = 0
	var end int64 = cppbridge.MaxPointsInChunk + 1

	var timeSeries []storagetest.TimeSeries
	for i := 0; i < int(end); i++ {
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
	seriesSet := query(s.T(), s.lss, s.ds, start, end, matcher)

	// Assert
	require.Equal(s.T(), expected, storagetest.TimeSeriesFromSeriesSet(seriesSet, false))
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
	seriesSet := query(s.T(), s.lss, s.ds, start, end, matcher)

	// Assert
	require.Equal(s.T(), expected, storagetest.TimeSeriesFromSeriesSet(seriesSet, false))
}

func (s *SeriesSetTestSuite) TestQueryMergedSeriesSets() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	var start int64 = 0
	var end int64 = 1000

	timeSeries1 := []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test1"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test2"),
			Samples: []cppbridge.Sample{
				{Timestamp: 1, Value: 2},
			},
		},
	}

	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, timeSeries1...)

	anotherLss := shard.NewLSS()
	anotherDs := shard.NewDataStorage()

	timeSeries2 := []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test3"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test4"),
			Samples: []cppbridge.Sample{
				{Timestamp: 1, Value: 2},
			},
		},
	}

	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(anotherLss, anotherDs, timeSeries2...)
	expected := append(timeSeries1, timeSeries2...)
	// Act
	seriesSet1 := query(s.T(), s.lss, s.ds, start, end, matcher)
	seriesSet2 := query(s.T(), anotherLss, anotherDs, start, end, matcher)

	// Assert
	require.Equal(
		s.T(),
		expected,
		storagetest.TimeSeriesFromSeriesSet(
			storage.NewMergeSeriesSet([]storage.SeriesSet{seriesSet1, seriesSet2}, storage.ChainedSeriesMerge), false),
	)
}

func (s *SeriesSetTestSuite) TestSeriesSeek() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	var start int64 = 0
	var end int64 = 1000

	expected := s.timeSeries[:4]
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, s.timeSeries[:4]...)
	seriesSet := query(s.T(), s.lss, s.ds, start, end, matcher)
	require.True(s.T(), seriesSet.Next())
	series := seriesSet.At()
	require.Equal(s.T(), expected[0].Labels, series.Labels())
	var iterator chunkenc.Iterator
	iterator = series.Iterator(iterator)
	index := 2
	// Act
	result := iterator.Seek(expected[index].Samples[0].Timestamp)

	// Assert
	require.Equal(s.T(), chunkenc.ValFloat, result)
	ts, v := iterator.At()
	require.Equal(s.T(), ts, expected[index].Samples[0].Timestamp)
	require.Equal(s.T(), v, expected[index].Samples[0].Value)

	for {
		index++
		if index > len(expected)-1 {
			return
		}

		iterator.Next()
		ts, v = iterator.At()
		require.Equal(s.T(), ts, expected[index].Samples[0].Timestamp)
		require.Equal(s.T(), v, expected[index].Samples[0].Value)
	}
}

func (s *SeriesSetTestSuite) TestSeriesSeekOutOfRange() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	var start int64 = 0
	var end int64 = 1000

	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, s.timeSeries[:4]...)
	seriesSet := query(s.T(), s.lss, s.ds, start, end, matcher)
	require.True(s.T(), seriesSet.Next())
	series := seriesSet.At()
	var iterator chunkenc.Iterator
	iterator = series.Iterator(iterator)

	// Act
	result := iterator.Seek(end)

	// Assert
	require.Equal(s.T(), chunkenc.ValNone, result)
}
