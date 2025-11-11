package querier_test

import (
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"testing"
)

type InstantSeriesTestSuite struct {
	suite.Suite

	lss                         *shard.LSS
	ds                          *shard.DataStorage
	data                        []storagetest.TimeSeries
	valueNotFoundTimestampValue int64
}

func TestInstantSeriesTestSuite(t *testing.T) {
	suite.Run(t, new(InstantSeriesTestSuite))
}

func (s *InstantSeriesTestSuite) SetupTest() {
	s.lss = shard.NewLSS()
	s.ds = shard.NewDataStorage()
	s.data = []storagetest.TimeSeries{
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
				{Timestamp: 12, Value: 2},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test_2"),
			Samples: []cppbridge.Sample{
				{Timestamp: 10, Value: 0},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test_2"),
			Samples: []cppbridge.Sample{
				{Timestamp: 11, Value: 1},
			},
		},
		{
			Labels: labels.FromStrings("__name__", "metric", "job", "test_2"),
			Samples: []cppbridge.Sample{
				{Timestamp: 12, Value: 2},
			},
		},
	}

	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, s.data...)
}

func (s *InstantSeriesTestSuite) TestSuccess() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	targetTimestamp := int64(11)
	expected := []storagetest.TimeSeries{s.data[1], s.data[4]}

	// Act
	seriesSet, err := storagetest.InstantQuery(s.lss, s.ds, targetTimestamp, s.valueNotFoundTimestampValue, matcher)

	// Assert
	require.NoError(s.T(), err)
	require.Equal(s.T(), expected, storagetest.TimeSeriesFromSeriesSet(seriesSet, false))
}

func (s *InstantSeriesTestSuite) TestEmptyResult() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	targetTimestamp := int64(4)
	expected := []storagetest.TimeSeries{}

	// Act
	seriesSet, err := storagetest.InstantQuery(s.lss, s.ds, targetTimestamp, s.valueNotFoundTimestampValue, matcher)

	// Assert
	require.NoError(s.T(), err)
	require.Equal(s.T(), expected, storagetest.TimeSeriesFromSeriesSet(seriesSet, false))
}

func (s *InstantSeriesTestSuite) TestBigTargetTimestamp() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	targetTimestamp := int64(1000000)
	expected := []storagetest.TimeSeries{s.data[2], s.data[5]}

	// Act
	seriesSet, err := storagetest.InstantQuery(s.lss, s.ds, targetTimestamp, s.valueNotFoundTimestampValue, matcher)

	// Assert
	require.NoError(s.T(), err)
	require.Equal(s.T(), expected, storagetest.TimeSeriesFromSeriesSet(seriesSet, false))
}
