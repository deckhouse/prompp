package querier

import (
	"slices"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/relabeler/head"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/context"
)

const (
	numberOfShards = 2
	maxSegmentSize = 1024

	relabelerName = "relabeler"
)

type QuerierTestSuite struct {
	suite.Suite
	context context.Context
	tmpDir  string
	head    *head.Head
}

func (s *QuerierTestSuite) SetupTest() {
	s.context = context.Background()
	s.tmpDir = s.T().TempDir()

	s.createHead()
}

func (s *QuerierTestSuite) createHead() {
	var err error
	s.head, err = head.Create(
		"test_head_id",
		0,
		s.tmpDir,
		[]*config.InputRelabelerConfig{
			{
				Name: relabelerName,
			},
		},
		numberOfShards,
		maxSegmentSize,
		head.NoOpLastAppendedSegmentIDSetter{},
		prometheus.DefaultRegisterer,
		1,
	)
	s.NoError(err)
}

func (s *QuerierTestSuite) fillHead(timeseries []model.TimeSeries) {
	tsd := relabeler.NewTimeSeriesDataSlice(timeseries)
	hx, err := (cppbridge.HashdexFactory{}).GoModel(tsd.TimeSeries(), cppbridge.DefaultWALHashdexLimits())
	s.NoError(err)

	_, _, err = s.head.Append(
		s.context,
		&relabeler.IncomingData{Hashdex: hx, Data: &tsd},
		cppbridge.NewState(s.head.NumberOfShards()),
		relabelerName,
		true)
	s.NoError(err)
}

func labelSetFromLabels(labels labels.Labels) model.LabelSet {
	simpleLabels := make([]model.SimpleLabel, labels.Len())
	for i, label := range labels {
		simpleLabels[i] = model.SimpleLabel{Name: label.Name, Value: label.Value}
	}

	return model.LabelSetFromSlice(simpleLabels)
}

func timeseriesFromSeriesSet(seriesSet storage.SeriesSet) []model.TimeSeries {
	var timeseries []model.TimeSeries
	for seriesSet.Next() {
		series := seriesSet.At()

		chunkIterator := series.Iterator(nil)
		for chunkIterator.Next() != chunkenc.ValNone {
			ts, v := chunkIterator.At()
			timeseries = append(timeseries, model.TimeSeries{
				LabelSet:  labelSetFromLabels(series.Labels()),
				Timestamp: uint64(ts),
				Value:     v,
			})
		}
	}

	return timeseries
}

func TestQuerierTestSuite(t *testing.T) {
	suite.Run(t, new(QuerierTestSuite))
}

func (s *QuerierTestSuite) TestRangeQuery() {
	// Arrange
	ls1 := model.NewLabelSetBuilder().Set("__name__", "metric").Set("job", "test").Build()
	ls2 := model.NewLabelSetBuilder().Set("__name__", "metric").Set("job", "test2").Build()
	timeseries := []model.TimeSeries{
		{
			LabelSet:  ls1,
			Timestamp: 0,
			Value:     1,
		},
		{
			LabelSet:  ls2,
			Timestamp: 0,
			Value:     10,
		},
	}
	s.fillHead(timeseries)

	q := NewQuerier(s.head, NoOpShardedDeduplicatorFactory(), 0, 2, nil, nil)
	defer func() { _ = q.Close() }()
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")

	// Act
	seriesSet := q.Select(s.context, false, nil, matcher)

	// Assert
	s.Equal(timeseries, timeseriesFromSeriesSet(seriesSet))
}

func (s *QuerierTestSuite) TestRangeQueryWithDataStorageLoading() {
	// Arrange
	ls1 := model.NewLabelSetBuilder().Set("__name__", "metric").Set("job", "test").Build()
	ls2 := model.NewLabelSetBuilder().Set("__name__", "metric").Set("job", "test2").Build()
	ls1Timeseries := []model.TimeSeries{
		{
			LabelSet:  ls1,
			Timestamp: 0,
			Value:     0,
		},
		{
			LabelSet:  ls1,
			Timestamp: 1,
			Value:     1,
		},
		{
			LabelSet:  ls1,
			Timestamp: 2,
			Value:     2,
		},
	}
	ls2Timeseries := []model.TimeSeries{
		{
			LabelSet:  ls2,
			Timestamp: 0,
			Value:     10,
		},
		{
			LabelSet:  ls2,
			Timestamp: 1,
			Value:     11,
		},
		{
			LabelSet:  ls2,
			Timestamp: 2,
			Value:     12,
		},
	}
	s.fillHead(ls1Timeseries)
	s.fillHead(ls2Timeseries)

	ls1TimeseriesAfterUnload := []model.TimeSeries{
		{
			LabelSet:  ls1,
			Timestamp: 3,
			Value:     3,
		},
	}
	ls2TimeseriesAfterUnload := []model.TimeSeries{
		{
			LabelSet:  ls2,
			Timestamp: 3,
			Value:     13,
		},
	}

	q := NewQuerier(s.head, NoOpShardedDeduplicatorFactory(), 0, 3, nil, nil)
	defer func() { _ = q.Close() }()
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")

	// Act
	q.head.UnloadUnusedSeriesData()
	s.fillHead(ls1TimeseriesAfterUnload)
	s.fillHead(ls2TimeseriesAfterUnload)
	seriesSet := q.Select(s.context, false, nil, matcher)

	// Assert
	s.Equal(slices.Concat(ls1Timeseries, ls1TimeseriesAfterUnload, ls2Timeseries, ls2TimeseriesAfterUnload), timeseriesFromSeriesSet(seriesSet))
}

func (s *QuerierTestSuite) TestInstantQuery() {
	// Arrange
	ls1 := model.NewLabelSetBuilder().Set("__name__", "metric").Set("job", "test").Build()
	ls2 := model.NewLabelSetBuilder().Set("__name__", "metric").Set("job", "test2").Build()
	timeseries := []model.TimeSeries{
		{
			LabelSet:  ls1,
			Timestamp: 0,
			Value:     1,
		},
		{
			LabelSet:  ls2,
			Timestamp: 0,
			Value:     10,
		},
	}
	s.fillHead(timeseries)

	q := NewQuerier(s.head, NoOpShardedDeduplicatorFactory(), 0, 0, nil, nil)
	defer func() { _ = q.Close() }()
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")

	// Act
	seriesSet := q.Select(s.context, false, nil, matcher)

	// Assert
	s.Equal(timeseries, timeseriesFromSeriesSet(seriesSet))
}

func (s *QuerierTestSuite) TestInstantQueryWithDataStorageLoading() {
	// Arrange
	ls1 := model.NewLabelSetBuilder().Set("__name__", "metric").Set("job", "test").Build()
	ls2 := model.NewLabelSetBuilder().Set("__name__", "metric").Set("job", "test2").Build()
	ls1Timeseries := []model.TimeSeries{
		{
			LabelSet:  ls1,
			Timestamp: 0,
			Value:     0,
		},
		{
			LabelSet:  ls1,
			Timestamp: 1,
			Value:     1,
		},
		{
			LabelSet:  ls1,
			Timestamp: 2,
			Value:     2,
		},
	}
	ls2Timeseries := []model.TimeSeries{
		{
			LabelSet:  ls2,
			Timestamp: 0,
			Value:     10,
		},
		{
			LabelSet:  ls2,
			Timestamp: 1,
			Value:     11,
		},
		{
			LabelSet:  ls2,
			Timestamp: 2,
			Value:     12,
		},
	}
	s.fillHead(ls1Timeseries)
	s.fillHead(ls2Timeseries)

	ls1TimeseriesAfterUnload := []model.TimeSeries{
		{
			LabelSet:  ls1,
			Timestamp: 3,
			Value:     3,
		},
	}
	ls2TimeseriesAfterUnload := []model.TimeSeries{
		{
			LabelSet:  ls2,
			Timestamp: 3,
			Value:     13,
		},
	}

	q := NewQuerier(s.head, NoOpShardedDeduplicatorFactory(), 0, 0, nil, nil)
	defer func() { _ = q.Close() }()
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")

	// Act
	q.head.UnloadUnusedSeriesData()
	s.fillHead(ls1TimeseriesAfterUnload)
	s.fillHead(ls2TimeseriesAfterUnload)
	seriesSet := q.Select(s.context, false, nil, matcher)

	// Assert
	s.Equal([]model.TimeSeries{
		{
			LabelSet:  ls1,
			Timestamp: 0,
			Value:     0,
		},
		{
			LabelSet:  ls2,
			Timestamp: 0,
			Value:     10,
		},
	}, timeseriesFromSeriesSet(seriesSet))
}
