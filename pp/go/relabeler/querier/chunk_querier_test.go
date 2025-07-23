package querier

import (
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/storage"
	"github.com/stretchr/testify/suite"
	"testing"
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
	labelSet     model.LabelSet
	samplesCount []int
}

func getChunkSeriesSetInfo(chunkSeriesSet storage.ChunkSeriesSet) []chunkSeriesSetInfo {
	var info []chunkSeriesSetInfo

	for chunkSeriesSet.Next() {
		chunkSeries := chunkSeriesSet.At()
		item := chunkSeriesSetInfo{
			labelSet: labelSetFromLabels(chunkSeries.Labels()),
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
	ls1 := model.NewLabelSetBuilder().Set("__name__", "metric").Set("job", "test").Build()
	ls2 := model.NewLabelSetBuilder().Set("__name__", "metric").Set("job", "test2").Build()
	ls3 := model.NewLabelSetBuilder().Set("__name__", "metric").Set("job", "test3").Build()
	timeseries := []model.TimeSeries{
		{
			LabelSet:  ls1,
			Timestamp: 0,
			Value:     1,
		},
		{
			LabelSet:  ls1,
			Timestamp: 1,
			Value:     1,
		},
		{
			LabelSet:  ls2,
			Timestamp: 0,
			Value:     10,
		},
		{
			LabelSet:  ls3,
			Timestamp: 10,
			Value:     10,
		},
	}
	s.fillHead(timeseries)

	q := NewChunkQuerier(s.head, NoOpShardedDeduplicatorFactory(), 0, 2, nil)
	defer func() { _ = q.Close() }()
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")

	// Act
	chunkSeriesSet := q.Select(s.context, false, nil, matcher)

	// Assert
	s.Equal([]chunkSeriesSetInfo{
		{labelSet: ls1, samplesCount: []int{2}},
		{labelSet: ls2, samplesCount: []int{1}},
	}, getChunkSeriesSetInfo(chunkSeriesSet))
}

func (s *ChunkQuerierTestSuite) TestSelectWithDataStorageLoading() {
	// Arrange
	ls1 := model.NewLabelSetBuilder().Set("__name__", "metric").Set("job", "test").Build()
	ls2 := model.NewLabelSetBuilder().Set("__name__", "metric").Set("job", "test2").Build()
	timeseries := []model.TimeSeries{
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
		{
			LabelSet:  ls1,
			Timestamp: 3,
			Value:     3,
		},
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
	s.fillHead(timeseries)

	q := NewChunkQuerier(s.head, NoOpShardedDeduplicatorFactory(), 0, 4, nil)
	defer func() { _ = q.Close() }()
	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric")

	// Act
	q.head.UnloadUnusedSeriesData()
	s.fillHead([]model.TimeSeries{
		{
			LabelSet:  ls1,
			Timestamp: 4,
			Value:     4,
		},
		{
			LabelSet:  ls2,
			Timestamp: 3,
			Value:     13,
		},
	})
	chunkSeriesSet := q.Select(s.context, false, nil, matcher)

	// Assert
	s.Equal([]chunkSeriesSetInfo{
		{labelSet: ls1, samplesCount: []int{5}},
		{labelSet: ls2, samplesCount: []int{4}},
	}, getChunkSeriesSetInfo(chunkSeriesSet))
}
