package querier_test

import (
	"testing"

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
)

type AggSeriesSetSuite struct {
	suite.Suite

	timeSeries []storagetest.TimeSeries
	lss        *shard.LSS
	ds         *shard.DataStorage
}

func TestAggSeriesSetSuite(t *testing.T) {
	suite.Run(t, new(AggSeriesSetSuite))
}

func (s *AggSeriesSetSuite) SetupTest() {
	s.lss = shard.NewLSS()
	s.ds = shard.NewDataStorage()

	s.timeSeries = []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test", "instance", "instance1"),
			Samples: []cppbridge.Sample{
				{Timestamp: 10, Value: 1},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test", "instance", "instance1"),
			Samples: []cppbridge.Sample{
				{Timestamp: 11, Value: 3},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test", "instance", "instance1"),
			Samples: []cppbridge.Sample{
				{Timestamp: 12, Value: 5},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test", "instance", "instance1"),
			Samples: []cppbridge.Sample{
				{Timestamp: 13, Value: 7},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test2", "instance", "instance2"),
			Samples: []cppbridge.Sample{
				{Timestamp: 10, Value: 1},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test2", "instance", "instance2"),
			Samples: []cppbridge.Sample{
				{Timestamp: 11, Value: 2},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test2", "instance", "instance2"),
			Samples: []cppbridge.Sample{
				{Timestamp: 12, Value: 4},
			},
		},
	}
}

func (s *AggSeriesSetSuite) query(
	lss *shard.LSS,
	ds *shard.DataStorage,
	start, end, downsamplingMs int64,
	hints *storage.SelectHints,
	matchers ...model.LabelMatcher,
) *querier.AggSeriesSet {
	selector, snapshot, err := lss.QuerySelector(0, matchers)
	s.Require().NoError(err)

	if selector == 0 || snapshot == nil {
		return &querier.AggSeriesSet{}
	}

	lssQueryResult := snapshot.Query(selector)
	if lssQueryResult.Status() == cppbridge.LSSQueryStatusNoMatch {
		return &querier.AggSeriesSet{}
	}

	dsQueryResult := ds.Query(cppbridge.DataStorageQuery{
		StartTimestampMs: start,
		EndTimestampMs:   end,
		LabelSetIDs:      lssQueryResult.IDs(),
	}, downsamplingMs, hints)

	nameIDs := make([]uint32, len(hints.Grouping))
	lss.LabelNameToIDs(hints.Grouping, nameIDs)
	seriesGroups := lss.GroupSeriesByLabelNames(lssQueryResult.IDs(), nameIDs)

	s.Require().Equal(cppbridge.DataStorageQueryStatusSuccess, dsQueryResult.Status)
	return querier.NewAggSeriesSet(
		dsQueryResult.SerializedData,
		snapshot,
		seriesGroups,
		start,
		end,
		hints.Grouping,
		"head_id",
		0,
	)
}

func (s *AggSeriesSetSuite) TestQuerySum() {
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
			Labels: labels.FromStrings("__head_id", "head_id", "__shard_id", "0"),
			Samples: []cppbridge.Sample{
				{Timestamp: 10, Value: 1},
				{Timestamp: 11, Value: 5},
				{Timestamp: 12, Value: 9},
				{Timestamp: 13, Value: 7},
			},
		},
	}

	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, s.timeSeries...)

	// Act
	seriesSet := s.query(s.lss, s.ds, start, end, cppbridge.NoDownsampling, hints, matcher)

	// Assert
	s.Require().Equal(expected, storagetest.TimeSeriesFromSeriesSet(seriesSet, true))
}

//
//
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

	seriesGroups := head.lsses[0].GroupSeriesByLabelNames(seriesIDs, nameIDs)
	t.Log("seriesGroups", seriesGroups.Groups)

	// t.Log("createLabelSet", aggLabelSetCtor(snapshot, hints.Grouping, "headID", seriesGroups.Groups[0][0], shardID))
	// t.Log("createLabelSet", aggLabelSetCtor(snapshot, hints.Grouping, "headID", seriesGroups.Groups[1][0], shardID))

	result := head.dss[shardID].Query(
		cppbridge.DataStorageQuery{
			StartTimestampMs: hints.Start,
			EndTimestampMs:   hints.End,
			LabelSetIDs:      seriesIDs,
		},
		cppbridge.NoDownsampling,
		hints,
	)

	ss := querier.NewAggSeriesSet(
		result.SerializedData,
		snapshot,
		seriesGroups,
		hints.Start,
		hints.End,
		hints.Grouping,
		"headID",
		shardID,
	)

	var it chunkenc.Iterator
	for ss.Next() {
		s := ss.At()
		t.Log(s.Labels())
		it = s.Iterator(it)
		t.Log(it.Next())
		for it.Next() != chunkenc.ValNone {
			ts, v := it.At()
			t.Log(ts, v)
		}
	}
}
