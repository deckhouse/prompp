package querier

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/headtest"
	"github.com/prometheus/prometheus/storage"
	"github.com/stretchr/testify/suite"
)

type ChunkQuerierTestSuite struct {
	QuerierTestSuite
}

func (s *ChunkQuerierTestSuite) SetupTest() {
	s.QuerierTestSuite.SetupTest()
}

func TestChunkQuerierTestSuite(t *testing.T) {
	suite.Run(t, new(ChunkQuerierTestSuite))
}

type chunkSeriesSetInfo struct {
	labelSet     labels.Labels
	samplesCount []int
}

func getChunkSeriesSetInfo(chunkSeriesSet storage.ChunkSeriesSet) []chunkSeriesSetInfo {
	var info []chunkSeriesSetInfo

	for chunkSeriesSet.Next() {
		chunkSeries := chunkSeriesSet.At()
		item := chunkSeriesSetInfo{
			labelSet: chunkSeries.Labels(),
		}

		for it := chunkSeries.Iterator(nil); it.Next(); {
			item.samplesCount = append(item.samplesCount, it.At().Chunk.NumSamples())
		}

		info = append(info, item)
	}

	return info
}

func (s *ChunkQuerierTestSuite) TestSelect() {
	// Arrange
	timeSeries := []headtest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
				{Timestamp: 1, Value: 1},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test2"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 10},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test3"),
			Samples: []cppbridge.Sample{
				{Timestamp: 10, Value: 10},
			},
		},
	}
	s.fillHead(timeSeries)

	q := NewChunkQuerier(s.head, NoOpShardedDeduplicatorFactory(), 0, 2, nil)
	defer func() { _ = q.Close() }()
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")

	// Act
	chunkSeriesSet := q.Select(s.context, false, nil, matcher)

	// Assert
	actual := getChunkSeriesSetInfo(chunkSeriesSet)
	expected := []chunkSeriesSetInfo{
		{labelSet: timeSeries[0].Labels, samplesCount: []int{2}},
		{labelSet: timeSeries[1].Labels, samplesCount: []int{1}},
	}
	s.Require().Equal(len(expected), len(actual))
	for i := range expected {
		s.True(labels.Equal(expected[i].labelSet, actual[i].labelSet))
		s.Equal(expected[i].samplesCount, actual[i].samplesCount)
	}
}

func (s *ChunkQuerierTestSuite) TestSelectWithDataStorageLoading() {
	// Arrange
	timeSeries := []headtest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 0},
				{Timestamp: 1, Value: 1},
				{Timestamp: 2, Value: 2},
				{Timestamp: 3, Value: 3},
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
	s.fillHead(timeSeries)

	q := NewChunkQuerier(s.head, NoOpShardedDeduplicatorFactory(), 0, 4, nil)
	defer func() { _ = q.Close() }()
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")

	// Act
	s.head.UnloadUnusedSeriesData()
	s.fillHead([]headtest.TimeSeries{
		{
			Labels: timeSeries[0].Labels,
			Samples: []cppbridge.Sample{
				{Timestamp: 4, Value: 4},
			},
		},
		{
			Labels: timeSeries[1].Labels,
			Samples: []cppbridge.Sample{
				{Timestamp: 3, Value: 13},
			},
		},
	})
	chunkSeriesSet := q.Select(s.context, false, nil, matcher)

	// Assert
	actual := getChunkSeriesSetInfo(chunkSeriesSet)
	expected := []chunkSeriesSetInfo{
		{labelSet: timeSeries[0].Labels, samplesCount: []int{5}},
		{labelSet: timeSeries[1].Labels, samplesCount: []int{4}},
	}
	s.Require().Equal(len(expected), len(actual))
	for i := range expected {
		s.True(labels.Equal(expected[i].labelSet, actual[i].labelSet))
		s.Equal(expected[i].samplesCount, actual[i].samplesCount)
	}
}
