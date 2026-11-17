package upsampler_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/upsampler"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
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
	ss := upsampler.NewSeriesSet(baseSS, 60000, 60000, upsampler.CounterFunc)

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

	ss := upsampler.NewSeriesSet(baseSS, 60000, 60000, upsampler.CounterFunc)

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

	ss := upsampler.NewSeriesSet(baseSS, 60000, 60000, upsampler.CounterFunc)

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

	ss := upsampler.NewSeriesSet(baseSS, 60000, 60000, upsampler.CounterFunc)

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

	ss := upsampler.NewSeriesSet(baseSS, 60000, 60000, upsampler.CounterFunc)

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

	ss := upsampler.NewSeriesSet(baseSS, 60000, 60000, upsampler.CounterFunc)

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

	ss := upsampler.NewSeriesSet(baseSS, 60000, 60000, upsampler.CounterFunc)

	// Iterate through all series
	count := 0
	for ss.Next() {
		series := ss.At()
		s.NotNil(series)
		count++
	}

	s.Equal(2, count)
}

func (s *SeriesSetSuite) TestSeriesSetMillisecondRangeNext() {
	for _, tc := range []struct {
		name       string
		rangeMS    int64
		timestamps []int64
		values     []float64
	}{
		{name: "one_millisecond", rangeMS: 1, timestamps: []int64{100, 102}, values: []float64{10, 30}},
		{name: "two_milliseconds", rangeMS: 2, timestamps: []int64{100, 101, 102}, values: []float64{10, 20, 30}},
	} {
		s.Run(tc.name, func() {
			// Arrange
			it := s.newRangeIterator(tc.rangeMS)

			// Act
			for i, timestamp := range tc.timestamps {
				valueType := it.Next()

				// Assert
				s.Require().Equal(chunkenc.ValFloat, valueType)
				t, v := it.At()
				s.Require().Equal(timestamp, t)
				s.Require().Equal(tc.values[i], v)
			}

			s.Require().Equal(chunkenc.ValNone, it.Next())
			s.Require().NoError(it.Err())
		})
	}
}

func (s *SeriesSetSuite) TestSeriesSetOneMillisecondRangeSeek() {
	for _, tc := range []struct {
		name        string
		initialized bool
	}{
		{name: "initial"},
		{name: "after_next", initialized: true},
	} {
		s.Run(tc.name, func() {
			// Arrange
			it := s.newRangeIterator(1)
			if tc.initialized {
				s.Require().Equal(chunkenc.ValFloat, it.Next())
			}

			// Act
			valueType := it.Seek(101)

			// Assert
			s.Require().Equal(chunkenc.ValFloat, valueType)
			t, v := it.At()
			s.Require().Equal(int64(102), t)
			s.Require().Equal(float64(30), v)
			s.Require().Equal(chunkenc.ValNone, it.Next())
			s.Require().NoError(it.Err())
		})
	}
}

// TestSeriesSetParametersOverflowingThresholds tests that a range or a resolution too large
// for the 32-bit gap thresholds disarms synthesis instead of wrapping around into a step that
// means nothing: the samples pass through as they are.
func (s *SeriesSetSuite) TestSeriesSetParametersOverflowingThresholds() {
	const dayMS = int64(24 * 60 * 60 * 1000)

	samples := []struct {
		t int64
		v float64
	}{
		{t: 0, v: 0.0},
		{t: 10 * dayMS, v: 10.0},
	}

	// A 100-day range asks for a 50-day step, which does not fit into the threshold.
	ss := upsampler.NewSeriesSet(newSingleSeriesSet(newMockIterator(samples)), 100*dayMS, 50*dayMS, upsampler.CounterFunc)

	s.Require().True(ss.Next())
	it := ss.At().Iterator(nil)

	var result []struct {
		t int64
		v float64
	}
	for it.Next() == chunkenc.ValFloat {
		t, v := it.At()
		result = append(result, struct {
			t int64
			v float64
		}{t, v})
	}

	s.Require().Equal(samples, result)
}

func (s *SeriesSetSuite) newRangeIterator(rangeMS int64) chunkenc.Iterator {
	c := chunkenc.NewXORChunk()
	appender, err := c.Appender()
	s.Require().NoError(err)
	appender.Append(100, 10)
	appender.Append(102, 30)

	ss := upsampler.NewSeriesSet(newSingleSeriesSet(c.Iterator(nil)), rangeMS, 1, upsampler.CounterFunc)
	s.Require().True(ss.Next())

	return ss.At().Iterator(nil)
}
