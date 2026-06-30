package querier_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/prometheus/prometheus/storage"
)

type AggrSeriesSetSuite struct {
	suite.Suite

	lss *shard.LSS
	ds  *shard.DataStorage
}

func TestAggrSeriesSetSuite(t *testing.T) {
	suite.Run(t, new(AggrSeriesSetSuite))
}

func (s *AggrSeriesSetSuite) SetupTest() {
	s.lss = shard.NewLSS()
	s.ds = shard.NewDataStorage()

	timeSeries := []storagetest.TimeSeries{
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test", "instance", "instance1"),
			Samples: []cppbridge.Sample{{Timestamp: 10, Value: 1}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test", "instance", "instance1"),
			Samples: []cppbridge.Sample{{Timestamp: 11, Value: 5}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test", "instance", "instance1"),
			Samples: []cppbridge.Sample{{Timestamp: 12, Value: 3}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test", "instance", "instance1"),
			Samples: []cppbridge.Sample{{Timestamp: 13, Value: 7}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test2", "instance", "instance2"),
			Samples: []cppbridge.Sample{{Timestamp: 10, Value: 1}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test2", "instance", "instance2"),
			Samples: []cppbridge.Sample{{Timestamp: 11, Value: 4}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test2", "instance", "instance2"),
			Samples: []cppbridge.Sample{{Timestamp: 12, Value: 2}},
		},
	}

	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, timeSeries...)
}

func (s *AggrSeriesSetSuite) query(
	lss *shard.LSS,
	ds *shard.DataStorage,
	start, end, downsamplingMs int64,
	hints *storage.SelectHints,
	matchers ...model.LabelMatcher,
) storage.SeriesSet {
	selector, snapshot, err := lss.QuerySelector(0, matchers)
	s.Require().NoError(err)

	if selector == 0 || snapshot == nil {
		return &querier.AggrSeriesSet{}
	}

	lssQueryResult := snapshot.Query(selector)
	if lssQueryResult.Status() == cppbridge.LSSQueryStatusNoMatch {
		return &querier.AggrSeriesSet{}
	}

	dsQueryResult := ds.Query(cppbridge.DataStorageQuery{
		StartTimestampMs: start,
		EndTimestampMs:   end,
		LabelSetIDs:      lssQueryResult.IDs(),
	}, downsamplingMs, hints)

	s.Require().Equal(cppbridge.DataStorageQueryStatusSuccess, dsQueryResult.Status)

	aggSS := querier.NewAggrSeriesSet(
		snapshot,
		dsQueryResult.SerializedData,
		lssQueryResult,
		start,
		end,
	)

	return aggSS
}

func (s *AggrSeriesSetSuite) TestMaxOverTimeFunc() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	var start int64
	var end int64 = 50
	hints := &storage.SelectHints{
		Start: 10,
		End:   13,
		Step:  3,
		Func:  "max_over_time",
		Range: 3,
	}

	expected := []storagetest.TimeSeries{
		{
			Labels:  labels.FromStrings("__name__", "metric", "instance", "instance1", "job", "test"),
			Samples: []cppbridge.Sample{{Timestamp: 11, Value: 5}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "instance", "instance2", "job", "test2"),
			Samples: []cppbridge.Sample{{Timestamp: 11, Value: 4}},
		},
	}

	// Act
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, hints, matcher)

	// Assert
	actual := storagetest.TimeSeriesFromSeriesSet(seriesSet, true)
	s.Require().Equal(len(expected), len(actual))
	s.Equal(expected, actual)
}

func (s *AggrSeriesSetSuite) TestMinOverTimeFunc() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	var start int64
	var end int64 = 50
	hints := &storage.SelectHints{
		Start: 10,
		End:   13,
		Step:  3,
		Func:  "min_over_time",
		Range: 3,
	}

	expected := []storagetest.TimeSeries{
		{
			Labels:  labels.FromStrings("__name__", "metric", "instance", "instance1", "job", "test"),
			Samples: []cppbridge.Sample{{Timestamp: 10, Value: 1}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "instance", "instance2", "job", "test2"),
			Samples: []cppbridge.Sample{{Timestamp: 10, Value: 1}},
		},
	}

	// Act
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, hints, matcher)

	// Assert
	actual := storagetest.TimeSeriesFromSeriesSet(seriesSet, true)
	s.Require().Equal(len(expected), len(actual))
	s.Equal(expected, actual)
}

func (s *AggrSeriesSetSuite) TestLastOverTimeFunc() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	var start int64
	var end int64 = 50
	hints := &storage.SelectHints{
		Start: 10,
		End:   13,
		Step:  3,
		Func:  "last_over_time",
		Range: 3,
	}

	expected := []storagetest.TimeSeries{
		{
			Labels:  labels.FromStrings("__name__", "metric", "instance", "instance1", "job", "test"),
			Samples: []cppbridge.Sample{{Timestamp: 12, Value: 3}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "instance", "instance2", "job", "test2"),
			Samples: []cppbridge.Sample{{Timestamp: 12, Value: 2}},
		},
	}

	// Act
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, hints, matcher)

	// Assert
	actual := storagetest.TimeSeriesFromSeriesSet(seriesSet, true)
	s.Require().Equal(len(expected), len(actual))
	s.Equal(expected, actual)
}
