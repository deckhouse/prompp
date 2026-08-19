package upsampler_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/upsampler"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

type SeriesSetSuite struct {
	suite.Suite
}

func TestSeriesSetSuite(t *testing.T) {
	suite.Run(t, new(SeriesSetSuite))
}

// TestNewSeriesSet tests SeriesSet creation.
func (s *SeriesSetSuite) TestNewSeriesSet() {
	baseSS := &mockSeriesSet{}
	ss := upsampler.NewSeriesSet(baseSS, 60000, 60000, true)

	s.NotNil(ss)
	s.IsType((*upsampler.SeriesSet)(nil), ss)
}

// TestSeriesSetNext delegates to base.
func (s *SeriesSetSuite) TestSeriesSetNext() {
	callCount := 0
	baseSS := &mockSeriesSet{
		nextFunc: func() bool {
			callCount++
			return true
		},
	}

	ss := upsampler.NewSeriesSet(baseSS, 60000, 60000, true)

	result := ss.Next()

	s.True(result)
	s.Equal(1, callCount)
}

// TestSeriesSetNextFalse tests when base returns false.
func (s *SeriesSetSuite) TestSeriesSetNextFalse() {
	baseSS := &mockSeriesSet{
		nextFunc: func() bool {
			return false
		},
	}

	ss := upsampler.NewSeriesSet(baseSS, 60000, 60000, true)

	result := ss.Next()

	s.False(result)
}

// TestSeriesSetAt returns wrapped Series.
func (s *SeriesSetSuite) TestSeriesSetAt() {
	baseSeries := &mockSeries{
		labels: labels.FromStrings("__name__", "test_metric", "job", "test"),
	}
	baseSS := &mockSeriesSet{
		atFunc: func() storage.Series {
			return baseSeries
		},
	}

	ss := upsampler.NewSeriesSet(baseSS, 60000, 60000, true)

	series := ss.At()

	s.NotNil(series)
	// Check that labels are preserved (Series.Labels() delegates to base)
	s.Equal(baseSeries.Labels(), series.Labels())
}

// TestSeriesSetErr delegates to base.
func (s *SeriesSetSuite) TestSeriesSetErr() {
	errCalled := false
	baseSS := &mockSeriesSet{
		errFunc: func() error {
			errCalled = true
			return nil
		},
	}

	ss := upsampler.NewSeriesSet(baseSS, 60000, 60000, true)

	_ = ss.Err()

	s.True(errCalled)
}

// TestSeriesSetWarnings delegates to base.
func (s *SeriesSetSuite) TestSeriesSetWarnings() {
	baseSS := &mockSeriesSet{
		warningsFunc: func() annotations.Annotations {
			return nil
		},
	}

	ss := upsampler.NewSeriesSet(baseSS, 60000, 60000, true)

	warnings := ss.Warnings()

	s.Nil(warnings)
}

// TestSeriesSetIteration tests full iteration flow.
func (s *SeriesSetSuite) TestSeriesSetIteration() {
	series1 := &mockSeries{
		labels: labels.FromStrings("__name__", "metric1"),
	}
	series2 := &mockSeries{
		labels: labels.FromStrings("__name__", "metric2"),
	}

	seriesList := []*mockSeries{series1, series2}
	currentIndex := -1

	baseSS := &mockSeriesSet{
		nextFunc: func() bool {
			currentIndex++
			return currentIndex < len(seriesList)
		},
		atFunc: func() storage.Series {
			if currentIndex >= 0 && currentIndex < len(seriesList) {
				return seriesList[currentIndex]
			}
			return nil
		},
	}

	ss := upsampler.NewSeriesSet(baseSS, 60000, 60000, true)

	// Iterate through all series
	count := 0
	for ss.Next() {
		series := ss.At()
		s.NotNil(series)
		count++
	}

	s.Equal(2, count)
}
