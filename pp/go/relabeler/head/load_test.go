package head

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/relabeler/querier"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/stretchr/testify/suite"
)

const (
	numberOfShards uint16 = 2

	maxSegmentSize uint32 = 1024

	headID                   = "test_head_id"
	transparentRelabelerName = "transparent_relabeler"
)

var cfgs = []*config.InputRelabelerConfig{
	{
		Name: transparentRelabelerName,
		RelabelConfigs: []*cppbridge.RelabelConfig{
			{
				SourceLabels: []string{"__name__"},
				Regex:        ".*",
				Action:       cppbridge.Keep,
			},
		},
	},
}

type HeadLoadSuite struct {
	suite.Suite
	dataDir string
	ctx     context.Context
}

func TestHeadLoadSuite(t *testing.T) {
	suite.Run(t, new(HeadLoadSuite))
}

func (s *HeadLoadSuite) SetupTest() {
	s.dataDir = s.createDataDirectory()
}

func (s *HeadLoadSuite) createDataDirectory() string {
	dataDir := filepath.Join(s.T().TempDir(), "data")
	s.Require().NoError(os.MkdirAll(dataDir, 0o777))
	return dataDir
}

func (s *HeadLoadSuite) createHead() *Head {
	h, err := Create(
		headID,
		0,
		s.dataDir,
		cfgs,
		numberOfShards,
		maxSegmentSize,
		NoOpLastAppendedSegmentIDSetter{},
		prometheus.DefaultRegisterer,
	)
	s.Require().NoError(err)

	return h
}

func (s *HeadLoadSuite) loadHead(unloadDataStorageInterval time.Duration) (*Head, bool, error) {
	loadedHead, corrupted, _, err := Load(
		headID,
		0,
		s.dataDir,
		cfgs,
		numberOfShards,
		maxSegmentSize,
		NoOpLastAppendedSegmentIDSetter{},
		prometheus.DefaultRegisterer,
		unloadDataStorageInterval,
	)

	return loadedHead, corrupted, err
}

func (s *HeadLoadSuite) mustLoadHead(unloadDataStorageInterval time.Duration) *Head {
	loadedHead, corrupted, err := s.loadHead(unloadDataStorageInterval)

	s.Require().NoError(err)
	s.False(corrupted)

	return loadedHead
}

type seriesData struct {
	labels  labels.Labels
	samples []cppbridge.Sample
}

func (s *seriesData) toTimeseries() []model.TimeSeries {
	lsBuilder := model.NewLabelSetBuilder()
	for i := range s.labels {
		lsBuilder.Add(s.labels[i].Name, s.labels[i].Value)
	}

	ls := lsBuilder.Build()

	timeseries := make([]model.TimeSeries, 0, len(s.samples))
	for i := range s.samples {
		timeseries = append(timeseries, model.TimeSeries{
			LabelSet:  ls,
			Timestamp: uint64(s.samples[i].Timestamp),
			Value:     s.samples[i].Value,
		})
	}

	return timeseries
}

func (s *HeadLoadSuite) appendTimeSeries(head *Head, seriesData []seriesData) {
	for i := range seriesData {
		tsd := relabeler.NewTimeSeriesDataSlice(seriesData[i].toTimeseries())
		hx, err := (cppbridge.HashdexFactory{}).GoModel(tsd.TimeSeries(), cppbridge.DefaultWALHashdexLimits())
		s.Require().NoError(err)

		incomingData := &relabeler.IncomingData{Hashdex: hx, Data: &tsd}

		_, _, err = head.Append(
			context.Background(),
			incomingData,
			cppbridge.NewState(head.NumberOfShards()),
			transparentRelabelerName,
			true,
		)
		s.Require().NoError(err)
	}
}

func (s *HeadLoadSuite) query(head *Head, matcher *labels.Matcher, minT, maxT int64) (result []seriesData) {
	q := querier.NewQuerier(head, querier.NoOpShardedDeduplicatorFactory(), minT, maxT, nil, nil)
	seriesSet := q.Select(context.Background(), false, nil, matcher)

	if seriesSet.Next() {
		series := seriesSet.At()
		item := seriesData{
			labels: seriesSet.At().Labels(),
		}

		chunkIterator := series.Iterator(nil)
		for chunkIterator.Next() != chunkenc.ValNone {
			ts, v := chunkIterator.At()
			item.samples = append(item.samples, cppbridge.Sample{
				Timestamp: ts,
				Value:     v,
			})
		}

		result = append(result, item)
	}

	_ = q.Close()
	return
}

func (s *HeadLoadSuite) TestErrorOpenShardFile() {
	// Arrange
	sourceHead := s.createHead()
	sourceHead.Stop()
	s.NoError(sourceHead.Close())

	s.Require().NoError(os.Remove(getShardWalFilename(s.dataDir, 0)))

	// Act
	head, corrupted, err := s.loadHead(0)

	// Assert
	s.True(corrupted)
	s.Require().NoError(err)
	s.Require().NoError(head.Close())
}

func (s *HeadLoadSuite) TestLoadSuccess() {
	// Arrange
	sourceHead := s.createHead()
	series := []seriesData{
		{
			labels: labels.Labels{{Name: "__name__", Value: "wal_metric"}},
			samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
				{Timestamp: 1, Value: 2},
				{Timestamp: 2, Value: 3},
			},
		},
	}
	s.appendTimeSeries(sourceHead, series)
	sourceHead.Stop()
	s.NoError(sourceHead.Close())

	// Act
	loadedHead := s.mustLoadHead(0)

	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "wal_metric")
	actual := s.query(loadedHead, matcher, 0, 2)
	err := loadedHead.Close()

	// Assert
	s.Equal(series, actual)
	s.Require().NoError(err)
}

func (s *HeadLoadSuite) TestAppendAfterLoad() {
	// Arrange
	sourceHead := s.createHead()
	series := []seriesData{
		{
			labels: labels.Labels{{Name: "__name__", Value: "wal_metric"}},
			samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
				{Timestamp: 1, Value: 2},
				{Timestamp: 2, Value: 3},
			},
		},
	}
	s.appendTimeSeries(sourceHead, series)
	sourceHead.Stop()
	s.NoError(sourceHead.Close())

	// Act
	loadedHead := s.mustLoadHead(0)
	s.appendTimeSeries(loadedHead, []seriesData{
		{
			labels: labels.Labels{{Name: "__name__", Value: "wal_metric"}},
			samples: []cppbridge.Sample{
				{Timestamp: 3, Value: 4},
			},
		},
	})

	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "wal_metric")
	actual := s.query(loadedHead, matcher, 0, 3)
	err := loadedHead.Close()

	// Assert
	series[0].samples = append(series[0].samples, cppbridge.Sample{Timestamp: 3, Value: 4})
	s.Equal(series, actual)
	s.Require().NoError(err)

}

func (s *HeadLoadSuite) TestLoadWithDataUnloading() {
	// Arrange
	sourceHead := s.createHead()
	series1 := []seriesData{
		{
			labels: labels.Labels{{Name: "__name__", Value: "wal_metric"}},
			samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
				{Timestamp: 1, Value: 2},
				{Timestamp: 2, Value: 3},
			},
		},
	}
	s.appendTimeSeries(sourceHead, series1)
	series2 := []seriesData{
		{
			labels: labels.Labels{{Name: "__name__", Value: "wal_metric"}},
			samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1},
				{Timestamp: 101, Value: 2},
				{Timestamp: 102, Value: 3},
			},
		},
	}
	s.appendTimeSeries(sourceHead, series2)

	sourceHead.UnloadUnusedSeriesData()
	sourceHead.Stop()
	s.NoError(sourceHead.Close())

	// Act
	loadedHead := s.mustLoadHead(100)

	matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "wal_metric")
	actual := s.query(loadedHead, matcher, 0, 3)
	err := loadedHead.Close()

	// Assert
	s.Equal(series1, actual)
	s.Require().NoError(err)
}
