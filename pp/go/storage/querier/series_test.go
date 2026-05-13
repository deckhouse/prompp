package querier_test

import (
	"math"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
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

func (s *SeriesSetTestSuite) query(
	lss *shard.LSS,
	ds *shard.DataStorage,
	start, end, downsamplingMs int64,
	hints *storage.SelectHints,
	matchers ...model.LabelMatcher,
) *querier.SeriesSet {
	selector, snapshot, err := lss.QuerySelector(0, matchers)
	require.NoError(s.T(), err)
	if selector == 0 || snapshot == nil {
		return &querier.SeriesSet{}
	}

	lssQueryResult := snapshot.Query(selector)
	if lssQueryResult.Status() == cppbridge.LSSQueryStatusNoMatch {
		return &querier.SeriesSet{}
	}

	dsQueryResult := ds.Query(cppbridge.DataStorageQuery{
		StartTimestampMs: start,
		EndTimestampMs:   end,
		LabelSetIDs:      lssQueryResult.IDs(),
	}, downsamplingMs, hints)

	require.Equal(s.T(), cppbridge.DataStorageQueryStatusSuccess, dsQueryResult.Status)
	return querier.NewSeriesSet(start, end, lssQueryResult, snapshot, dsQueryResult.SerializedData)
}

func (s *SeriesSetTestSuite) nextSample(iterator chunkenc.Iterator) cppbridge.Sample {
	s.Equal(chunkenc.ValFloat, iterator.Next())
	ts, v := iterator.At()
	return cppbridge.Sample{
		Timestamp: ts,
		Value:     v,
	}
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
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, &storage.SelectHints{}, matcher)

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
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, &storage.SelectHints{}, matcher)

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
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, &storage.SelectHints{}, matcher)

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
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, &storage.SelectHints{}, matcher)

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
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, &storage.SelectHints{}, matcher)

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
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, &storage.SelectHints{}, matcher)

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
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, &storage.SelectHints{}, matcher)

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
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, &storage.SelectHints{}, matcher)

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
	seriesSet1 := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, &storage.SelectHints{}, matcher)
	seriesSet2 := s.query(anotherLss, anotherDs, start, end, cppbridge.NoDownsampling, &storage.SelectHints{}, matcher)

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
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, &storage.SelectHints{}, matcher)
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

	index++
	require.Equal(s.T(), chunkenc.ValFloat, iterator.Next())
	ts, v = iterator.At()
	require.Equal(s.T(), ts, expected[index].Samples[0].Timestamp)
	require.Equal(s.T(), v, expected[index].Samples[0].Value)

	require.Equal(s.T(), chunkenc.ValNone, iterator.Next())
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
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, &storage.SelectHints{}, matcher)
	require.True(s.T(), seriesSet.Next())
	series := seriesSet.At()
	var iterator chunkenc.Iterator
	iterator = series.Iterator(iterator)

	// Act
	result := iterator.Seek(end)

	// Assert
	require.Equal(s.T(), chunkenc.ValNone, result)
}

func (s *SeriesSetTestSuite) TestSeriesParallelRead() {
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
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, &storage.SelectHints{}, matcher)
	seriesSlice := make([]storage.Series, 0, 2)
	require.True(s.T(), seriesSet.Next())
	seriesSlice = append(seriesSlice, seriesSet.At())
	require.True(s.T(), seriesSet.Next())
	seriesSlice = append(seriesSlice, seriesSet.At())
	require.False(s.T(), seriesSet.Next())
	var chunkIterator chunkenc.Iterator

	// Act
	timeSeriesFromSeries1 := storagetest.TimeSeriesFromSeries(seriesSlice[0], chunkIterator, false)
	timeSeriesFromSeries2 := storagetest.TimeSeriesFromSeries(seriesSlice[1], chunkIterator, false)

	// Assert
	require.Equal(s.T(), expected, append(timeSeriesFromSeries1, timeSeriesFromSeries2...))
}

func (s *SeriesSetTestSuite) TestSeriesResetIterator() {
	// Arrange
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, s.timeSeries...)

	var start int64 = 0
	var end int64 = 50
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, &storage.SelectHints{}, model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	})

	s.True(seriesSet.Next())
	iterator := seriesSet.At().Iterator(nil)
	s.Equal(chunkenc.ValFloat, iterator.Next())

	s.True(seriesSet.Next())

	// Act
	iterator = seriesSet.At().Iterator(iterator)

	// Assert
	s.Equal(cppbridge.Sample{Timestamp: 11, Value: 1}, s.nextSample(iterator))
	s.Equal(cppbridge.Sample{Timestamp: 12, Value: 2}, s.nextSample(iterator))
	s.Equal(chunkenc.ValNone, iterator.Next())
}

func (s *SeriesSetTestSuite) TestSeriesResetIteratorWithMinTimestamp() {
	// Arrange
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, s.timeSeries...)

	var start int64 = 12
	var end int64 = 50
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, &storage.SelectHints{}, model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	})

	s.True(seriesSet.Next())
	iterator := seriesSet.At().Iterator(nil)
	s.Equal(chunkenc.ValFloat, iterator.Next())

	s.True(seriesSet.Next())

	// Act
	iterator = seriesSet.At().Iterator(iterator)

	// Assert
	s.Equal(cppbridge.Sample{Timestamp: 12, Value: 2}, s.nextSample(iterator))
	s.Equal(chunkenc.ValNone, iterator.Next())
}

func (s *SeriesSetTestSuite) TestDownsampling() {
	// Arrange
	const Downsampling = 100
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 123, Value: 1.0},
				{Timestamp: 152, Value: 1.0},
				{Timestamp: 180, Value: 1.0},
				{Timestamp: 215, Value: 1.0},
				{Timestamp: 242, Value: 1.0},
				{Timestamp: 275, Value: 1.0},
				{Timestamp: 303, Value: 1.0},
			},
		},
	}...)

	// Act
	seriesSet := s.query(s.lss, s.ds, 0, 400, Downsampling, &storage.SelectHints{}, model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	})

	// Assert
	require.Equal(s.T(), []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 180, Value: 1.0},
				{Timestamp: 275, Value: 1.0},
				{Timestamp: 303, Value: 1.0},
			},
		},
	}, storagetest.TimeSeriesFromSeriesSet(seriesSet, true))
}

func (s *SeriesSetTestSuite) TestMinOverTimeFunc() {
	// Arrange
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1.0},
				{Timestamp: 150, Value: 2.0},
				{Timestamp: 200, Value: 3.0},
				{Timestamp: 250, Value: 0.0},
			},
		},
	}...)
	hints := &storage.SelectHints{
		Start: 100,
		End:   200,
		Func:  "min_over_time",
	}

	// Act
	seriesSet := s.query(s.lss, s.ds, 0, 400, cppbridge.NoDownsampling, hints, model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	})

	// Assert
	require.Equal(s.T(), []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 150, Value: 2.0},
			},
		},
	}, storagetest.TimeSeriesFromSeriesSet(seriesSet, true))
}

func (s *SeriesSetTestSuite) TestMaxOverTimeFunc() {
	// Arrange
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1.0},
				{Timestamp: 150, Value: 2.0},
				{Timestamp: 200, Value: 3.0},
				{Timestamp: 250, Value: 0.0},
			},
		},
	}...)
	hints := &storage.SelectHints{
		Start: 100,
		End:   200,
		Func:  "max_over_time",
	}

	// Act
	seriesSet := s.query(s.lss, s.ds, 0, 400, cppbridge.NoDownsampling, hints, model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	})

	// Assert
	require.Equal(s.T(), []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 200, Value: 3.0},
			},
		},
	}, storagetest.TimeSeriesFromSeriesSet(seriesSet, true))
}

func (s *SeriesSetTestSuite) TestLastOverTimeFunc() {
	// Arrange
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1.0},
				{Timestamp: 150, Value: 2.0},
				{Timestamp: 200, Value: 3.0},
				{Timestamp: 250, Value: math.Float64frombits(value.StaleNaN)},
				{Timestamp: 300, Value: 0.0},
			},
		},
	}...)
	hints := &storage.SelectHints{
		Start: 100,
		End:   250,
		Func:  "last_over_time",
	}

	// Act
	seriesSet := s.query(s.lss, s.ds, 0, 400, cppbridge.NoDownsampling, hints, model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	})

	// Assert
	require.Equal(s.T(), []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 200, Value: 3.0},
			},
		},
	}, storagetest.TimeSeriesFromSeriesSet(seriesSet, true))
}

func (s *SeriesSetTestSuite) TestSumOverTimeFunc() {
	// Arrange
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1.0},
				{Timestamp: 150, Value: 2.0},
				{Timestamp: 200, Value: 3.0},
				{Timestamp: 250, Value: math.Float64frombits(value.StaleNaN)},
				{Timestamp: 300, Value: 0.0},
			},
		},
	}...)
	hints := &storage.SelectHints{
		Start: 99,
		End:   250,
		Func:  "sum_over_time",
	}

	// Act
	seriesSet := s.query(s.lss, s.ds, 0, 400, cppbridge.NoDownsampling, hints, model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	})

	// Assert
	require.Equal(s.T(), []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 200, Value: 6.0},
			},
		},
	}, storagetest.TimeSeriesFromSeriesSet(seriesSet, true))
}

func (s *SeriesSetTestSuite) TestCountOverTimeFunc() {
	// Arrange
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1.0},
				{Timestamp: 150, Value: 2.0},
				{Timestamp: 200, Value: 3.0},
				{Timestamp: 250, Value: 0.0},
			},
		},
	}...)
	hints := &storage.SelectHints{
		Start: 100,
		End:   200,
		Func:  "count_over_time",
	}

	// Act
	seriesSet := s.query(s.lss, s.ds, 0, 400, cppbridge.NoDownsampling, hints, model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	})

	// Assert
	require.Equal(s.T(), []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 200, Value: 2.0},
			},
		},
	}, storagetest.TimeSeriesFromSeriesSet(seriesSet, true))
}

func (s *SeriesSetTestSuite) TestRateFunc() {
	// Arrange
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1.0},
				{Timestamp: 150, Value: 2.0},
				{Timestamp: 200, Value: 3.0},
				{Timestamp: 250, Value: math.Float64frombits(value.StaleNaN)},
				{Timestamp: 300, Value: 0.0},
			},
		},
	}...)
	hints := &storage.SelectHints{
		Start: 99,
		End:   250,
		Func:  "rate",
	}

	// Act
	seriesSet := s.query(s.lss, s.ds, 0, 400, cppbridge.NoDownsampling, hints, model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	})

	// Assert
	require.Equal(s.T(), []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1.0},
				{Timestamp: 200, Value: 3.0},
			},
		},
	}, storagetest.TimeSeriesFromSeriesSet(seriesSet, true))
}

func (s *SeriesSetTestSuite) TestIncreaseFunc() {
	// Arrange
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1.0},
				{Timestamp: 150, Value: 2.0},
				{Timestamp: 200, Value: 3.0},
				{Timestamp: 250, Value: math.Float64frombits(value.StaleNaN)},
				{Timestamp: 300, Value: 0.0},
			},
		},
	}...)
	hints := &storage.SelectHints{
		Start: 99,
		End:   250,
		Func:  "increase",
	}

	// Act
	seriesSet := s.query(s.lss, s.ds, 0, 400, cppbridge.NoDownsampling, hints, model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	})

	// Assert
	require.Equal(s.T(), []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1.0},
				{Timestamp: 200, Value: 3.0},
			},
		},
	}, storagetest.TimeSeriesFromSeriesSet(seriesSet, true))
}

func (s *SeriesSetTestSuite) TestChangesFunc() {
	// Arrange
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1.0},
				{Timestamp: 150, Value: 2.0},
				{Timestamp: 200, Value: 3.0},
				{Timestamp: 250, Value: math.Float64frombits(value.StaleNaN)},
				{Timestamp: 300, Value: 0.0},
			},
		},
	}...)
	hints := &storage.SelectHints{
		Start: 99,
		End:   250,
		Func:  "changes",
	}

	// Act
	seriesSet := s.query(s.lss, s.ds, 0, 400, cppbridge.NoDownsampling, hints, model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	})

	// Assert
	actual := storagetest.TimeSeriesFromSeriesSet(seriesSet, true)
	s.Equal(labels.FromStrings("__name__", "metric", "job", "test"), actual[0].Labels)
	s.Equal([]cppbridge.Sample{
		{Timestamp: 100, Value: 1.0},
		{Timestamp: 150, Value: 2.0},
		{Timestamp: 200, Value: 3.0},
	}, actual[0].Samples[:3])
	s.Equal(int64(250), actual[0].Samples[3].Timestamp)
	s.Equal(value.StaleNaN, math.Float64bits(actual[0].Samples[3].Value))
}

func (s *SeriesSetTestSuite) TestDeltaFunc() {
	// Arrange
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1.0},
				{Timestamp: 150, Value: 2.0},
				{Timestamp: 200, Value: 3.0},
				{Timestamp: 250, Value: math.Float64frombits(value.StaleNaN)},
				{Timestamp: 300, Value: 0.0},
			},
		},
	}...)
	hints := &storage.SelectHints{
		Start: 99,
		End:   300,
		Func:  "delta",
	}

	// Act
	seriesSet := s.query(s.lss, s.ds, 0, 400, cppbridge.NoDownsampling, hints, model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	})

	// Assert
	require.Equal(s.T(), []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1.0},
				{Timestamp: 300, Value: 0.0},
			},
		},
	}, storagetest.TimeSeriesFromSeriesSet(seriesSet, true))
}

func (s *SeriesSetTestSuite) TestIRateFunc() {
	// Arrange
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1.0},
				{Timestamp: 150, Value: 2.0},
				{Timestamp: 200, Value: 3.0},
				{Timestamp: 250, Value: math.Float64frombits(value.StaleNaN)},
				{Timestamp: 300, Value: 4.0},
			},
		},
	}...)
	hints := &storage.SelectHints{
		Start: 100,
		End:   300,
		Func:  "irate",
	}

	// Act
	seriesSet := s.query(s.lss, s.ds, 0, 400, cppbridge.NoDownsampling, hints, model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	})

	// Assert
	require.Equal(s.T(), []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 200, Value: 3.0},
				{Timestamp: 300, Value: 4.0},
			},
		},
	}, storagetest.TimeSeriesFromSeriesSet(seriesSet, true))
}

func (s *SeriesSetTestSuite) TestIDeltaFunc() {
	// Arrange
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1.0},
				{Timestamp: 150, Value: 2.0},
				{Timestamp: 200, Value: 3.0},
				{Timestamp: 250, Value: math.Float64frombits(value.StaleNaN)},
				{Timestamp: 300, Value: 4.0},
			},
		},
	}...)
	hints := &storage.SelectHints{
		Start: 100,
		End:   300,
		Func:  "idelta",
	}

	// Act
	seriesSet := s.query(s.lss, s.ds, 0, 400, cppbridge.NoDownsampling, hints, model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	})

	// Assert
	require.Equal(s.T(), []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 200, Value: 3.0},
				{Timestamp: 300, Value: 4.0},
			},
		},
	}, storagetest.TimeSeriesFromSeriesSet(seriesSet, true))
}

func (s *SeriesSetTestSuite) TestResetsFunc() {
	// Arrange
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 4.0},
				{Timestamp: 150, Value: 5.0},
				{Timestamp: 200, Value: 2.0},
				{Timestamp: 250, Value: math.Float64frombits(value.StaleNaN)},
				{Timestamp: 300, Value: 1.0},
			},
		},
	}...)
	hints := &storage.SelectHints{
		Start: 99,
		End:   300,
		Func:  "resets",
	}

	// Act
	seriesSet := s.query(s.lss, s.ds, 0, 400, cppbridge.NoDownsampling, hints, model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	})

	// Assert
	require.Equal(s.T(), []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 4.0},
				{Timestamp: 200, Value: 2.0},
				{Timestamp: 300, Value: 1.0},
			},
		},
	}, storagetest.TimeSeriesFromSeriesSet(seriesSet, true))
}
