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

type CrossSeriesSetSuite struct {
	suite.Suite

	timeSeries []storagetest.TimeSeries
	lss        *shard.LSS
	ds         *shard.DataStorage
}

func TestCrossSeriesSetSuite(t *testing.T) {
	suite.Run(t, new(CrossSeriesSetSuite))
}

func (s *CrossSeriesSetSuite) SetupTest() {
	s.lss = shard.NewLSS()
	s.ds = shard.NewDataStorage()

	s.timeSeries = []storagetest.TimeSeries{
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test", "instance", "instance1"),
			Samples: []cppbridge.Sample{{Timestamp: 10, Value: 1}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test", "instance", "instance1"),
			Samples: []cppbridge.Sample{{Timestamp: 11, Value: 3}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test", "instance", "instance1"),
			Samples: []cppbridge.Sample{{Timestamp: 12, Value: 5}},
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
			Samples: []cppbridge.Sample{{Timestamp: 11, Value: 2}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test2", "instance", "instance2"),
			Samples: []cppbridge.Sample{{Timestamp: 12, Value: 4}},
		},
	}
}

func (s *CrossSeriesSetSuite) query(
	lss *shard.LSS,
	ds *shard.DataStorage,
	start, end, downsamplingMs int64,
	hints *storage.SelectHints,
	matchers ...model.LabelMatcher,
) storage.SeriesSet {
	selector, snapshot, err := lss.QuerySelector(0, matchers)
	s.Require().NoError(err)

	if selector == 0 || snapshot == nil {
		return &querier.CrossSeriesSet{}
	}

	lssQueryResult := snapshot.Query(selector)
	if lssQueryResult.Status() == cppbridge.LSSQueryStatusNoMatch {
		return &querier.CrossSeriesSet{}
	}

	valueNotFoundTimestampValue := int64(0)
	timestamps := make([]int64, lssQueryResult.Len())
	ds.QueryFirstTimestamps(lssQueryResult.IDs(), timestamps, 0)

	sNaNSS := querier.NewStaleNaNSeriesSet(
		querier.NewStaleNaNSeriesSliceFromTimestamps(timestamps),
		lssQueryResult,
		snapshot,
		valueNotFoundTimestampValue,
	)

	dsQueryResult := ds.Query(cppbridge.DataStorageQuery{
		StartTimestampMs: start,
		EndTimestampMs:   end,
		LabelSetIDs:      lssQueryResult.IDs(),
	}, downsamplingMs, hints)

	nameIDs := make([]uint32, len(hints.Grouping))
	lss.LabelNameToIDs(hints.Grouping, nameIDs)
	seriesGroups := lss.GroupSeriesByLabelNames(lssQueryResult.IDs(), nameIDs)

	s.Require().Equal(cppbridge.DataStorageQueryStatusSuccess, dsQueryResult.Status)

	aggSS := querier.NewCrossSeriesSet(
		dsQueryResult.SerializedData,
		snapshot,
		seriesGroups,
		start,
		end,
		hints.Grouping,
		"head_id",
		0,
	)

	return querier.NewMergeShardSeriesSet([]storage.SeriesSet{sNaNSS, aggSS})
}

func (s *CrossSeriesSetSuite) TestQueryWithoutGrouping() {
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
		Step:  1,
		Func:  "sum",
		Range: 0,
	}

	expected := []storagetest.TimeSeries{
		{
			Labels:  labels.FromStrings("__head__shard_id", "head_id__0"),
			Samples: []cppbridge.Sample{},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test", "instance", "instance1"),
			Samples: []cppbridge.Sample{{Timestamp: 10, Value: math.Float64frombits(value.StaleNaN)}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test2", "instance", "instance2"),
			Samples: []cppbridge.Sample{{Timestamp: 10, Value: math.Float64frombits(value.StaleNaN)}},
		},
	}

	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, s.timeSeries...)

	// Act
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, hints, matcher)

	// Assert
	actual := storagetest.TimeSeriesFromSeriesSet(seriesSet, true)
	s.Require().Equal(len(expected), len(actual))
	s.Require().Equal(expected[0].Labels, actual[0].Labels)
	s.Require().Equal(expected[1].Labels, actual[1].Labels)
	s.Require().Equal(expected[2].Labels, actual[2].Labels)
}

func (s *CrossSeriesSetSuite) TestQueryGrouping_OneGroupingLabel() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	var start int64
	var end int64 = 50
	hints := &storage.SelectHints{
		Start:    10,
		End:      13,
		Step:     1,
		Func:     "sum",
		Range:    0,
		Grouping: []string{"job"},
	}

	expected := []storagetest.TimeSeries{
		{
			Labels:  labels.FromStrings("__head__shard_id", "head_id__0", "job", "test"),
			Samples: []cppbridge.Sample{},
		},
		{
			Labels:  labels.FromStrings("__head__shard_id", "head_id__0", "job", "test2"),
			Samples: []cppbridge.Sample{},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test", "instance", "instance1"),
			Samples: []cppbridge.Sample{{Timestamp: 10, Value: math.Float64frombits(value.StaleNaN)}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test2", "instance", "instance2"),
			Samples: []cppbridge.Sample{{Timestamp: 10, Value: math.Float64frombits(value.StaleNaN)}},
		},
	}

	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, s.timeSeries...)

	// Act
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, hints, matcher)

	// Assert
	actual := storagetest.TimeSeriesFromSeriesSet(seriesSet, true)
	s.Require().Equal(len(expected), len(actual))
	s.Require().Equal(expected[0].Labels, actual[0].Labels)
	s.Require().Equal(expected[1].Labels, actual[1].Labels)
	s.Require().Equal(expected[2].Labels, actual[2].Labels)
	s.Require().Equal(expected[3].Labels, actual[3].Labels)
}

func (s *CrossSeriesSetSuite) TestQueryGrouping_TwoGroupingLabels() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	var start int64
	var end int64 = 50
	hints := &storage.SelectHints{
		Start:    10,
		End:      13,
		Step:     1,
		Func:     "sum",
		Range:    0,
		Grouping: []string{"instance", "job"},
	}

	expected := []storagetest.TimeSeries{
		{
			Labels:  labels.FromStrings("__head__shard_id", "head_id__0", "job", "test", "instance", "instance1"),
			Samples: []cppbridge.Sample{},
		},
		{
			Labels:  labels.FromStrings("__head__shard_id", "head_id__0", "job", "test2", "instance", "instance2"),
			Samples: []cppbridge.Sample{},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test", "instance", "instance1"),
			Samples: []cppbridge.Sample{{Timestamp: 10, Value: math.Float64frombits(value.StaleNaN)}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test2", "instance", "instance2"),
			Samples: []cppbridge.Sample{{Timestamp: 10, Value: math.Float64frombits(value.StaleNaN)}},
		},
	}

	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, s.timeSeries...)

	// Act
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, hints, matcher)

	// Assert
	actual := storagetest.TimeSeriesFromSeriesSet(seriesSet, true)
	s.Require().Equal(len(expected), len(actual))
	s.Require().Equal(expected[0].Labels, actual[0].Labels)
	s.Require().Equal(expected[1].Labels, actual[1].Labels)
	s.Require().Equal(expected[2].Labels, actual[2].Labels)
	s.Require().Equal(expected[3].Labels, actual[3].Labels)
}

func (s *CrossSeriesSetSuite) TestQueryGrouping_TwoGroupingLabels_WithMissingGroupingLabel() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	var start int64
	var end int64 = 50
	hints := &storage.SelectHints{
		Start:    10,
		End:      13,
		Step:     1,
		Func:     "sum",
		Range:    0,
		Grouping: []string{"job", "instance", "head"},
	}

	expected := []storagetest.TimeSeries{
		{
			Labels:  labels.FromStrings("__head__shard_id", "head_id__0", "job", "test", "instance", "instance1"),
			Samples: []cppbridge.Sample{},
		},
		{
			Labels:  labels.FromStrings("__head__shard_id", "head_id__0", "job", "test2", "instance", "instance2"),
			Samples: []cppbridge.Sample{},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test", "instance", "instance1"),
			Samples: []cppbridge.Sample{{Timestamp: 10, Value: math.Float64frombits(value.StaleNaN)}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test2", "instance", "instance2"),
			Samples: []cppbridge.Sample{{Timestamp: 10, Value: math.Float64frombits(value.StaleNaN)}},
		},
	}

	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, s.timeSeries...)

	// Act
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, hints, matcher)

	// Assert
	actual := storagetest.TimeSeriesFromSeriesSet(seriesSet, true)
	s.Require().Equal(len(expected), len(actual))
	s.Require().Equal(expected[0].Labels, actual[0].Labels)
	s.Require().Equal(expected[1].Labels, actual[1].Labels)
}

//
// TODO DELETE
//

func TestAGGSS(t *testing.T) {
	hints := &storage.SelectHints{
		Start: 0,
		End:   6,
		Step:  1,
		Func:  "sum",
		Range: 1,
		Grouping: []string{
			"shard_id",
			"even_numbered",
			"head_id",
		},
		By: true,
	}

	shardID := uint16(0)
	head := makeHead(2, 10, 5)

	selector, snapshot, err := head.lsses[shardID].QuerySelector(
		shardID,
		[]model.LabelMatcher{{
			Name:        "__name__",
			Value:       "metric",
			MatcherType: model.MatcherTypeExactMatch,
		}},
	)
	require.NoError(t, err)

	lssQueryResult := snapshot.Query(selector)
	require.Equal(t, cppbridge.LSSQueryStatusMatch, lssQueryResult.Status())
	seriesIDs := lssQueryResult.IDs()
	t.Log("seriesIDs", seriesIDs)

	nameIDs := make([]uint32, len(hints.Grouping))
	t.Log("nameIDs empty", nameIDs)
	head.lsses[0].LabelNameToIDs(hints.Grouping, nameIDs)
	t.Log("nameIDs filled", nameIDs)

	seriesGroups := head.lsses[shardID].GroupSeriesByLabelNames(seriesIDs, nameIDs)
	t.Log("seriesGroups", seriesGroups.Groups)

	valueNotFoundTimestampValue := int64(0)
	timestamps := make([]int64, lssQueryResult.Len())
	head.dss[shardID].QueryFirstTimestamps(lssQueryResult.IDs(), timestamps, 0)
	t.Log("timestamps", timestamps)

	sNaNSS := querier.NewStaleNaNSeriesSet(
		querier.NewStaleNaNSeriesSliceFromTimestamps(timestamps),
		lssQueryResult,
		snapshot,
		valueNotFoundTimestampValue,
	)

	result := head.dss[shardID].Query(
		cppbridge.DataStorageQuery{
			StartTimestampMs: hints.Start,
			EndTimestampMs:   hints.End,
			LabelSetIDs:      seriesIDs,
		},
		cppbridge.NoDownsampling,
		hints,
	)

	aggSS := querier.NewCrossSeriesSet(
		result.SerializedData,
		snapshot,
		seriesGroups,
		hints.Start,
		hints.End,
		hints.Grouping,
		"headID",
		shardID,
	)

	ss := querier.NewMergeShardSeriesSet([]storage.SeriesSet{sNaNSS, aggSS})

	var it chunkenc.Iterator
	for ss.Next() {
		s := ss.At()
		t.Log("s.Labels()", s.Labels())
		it = s.Iterator(it)
		t.Log("it.Next()", it.Next())
		t.Log(it.At())
		for it.Next() != chunkenc.ValNone {
			ts, v := it.At()
			t.Log("ts", ts, "v", v)
		}
	}
}
