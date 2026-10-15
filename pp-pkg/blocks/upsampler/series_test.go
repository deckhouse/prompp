package upsampler_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/upsampler"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

type SeriesSuite struct {
	suite.Suite
}

func TestSeriesSuite(t *testing.T) {
	suite.Run(t, new(SeriesSuite))
}

func (s *SeriesSuite) TestSeriesIteratorNewCreation() {
	baseIterator := newMockIterator([]struct {
		t int64
		v float64
	}{
		{100, 1.0},
		{200, 2.0},
	})
	baseSeries := &mockSeries{
		labels: labels.FromStrings("__name__", "test_metric"),
		iteratorFunc: func(chunkenc.Iterator) chunkenc.Iterator {
			return baseIterator
		},
	}

	// Create Series through NewSeriesSet -> At() pattern since Series is not exported
	baseSeriesSet := &mockSeriesSet{
		atFunc: func() storage.Series {
			return baseSeries
		},
	}

	ss := upsampler.NewSeriesSet(baseSeriesSet, 60000, 60000, upsampler.CounterFunc)
	series := ss.At()

	// Get iterator without passing one (triggers NewIterator creation)
	it := series.Iterator(nil)

	// Should create a new Iterator (upsampler.Iterator wraps our mock)
	s.NotNil(it)
	_, ok := it.(*upsampler.Iterator)
	s.True(ok, "should return an upsampler.Iterator when passed nil")
}

func (s *SeriesSuite) TestSeriesIteratorReuse() {
	baseIterator := newMockIterator([]struct {
		t int64
		v float64
	}{
		{100, 1.0},
		{200, 2.0},
	})
	baseSeries := &mockSeries{
		labels: labels.FromStrings("__name__", "test_metric"),
		iteratorFunc: func(chunkenc.Iterator) chunkenc.Iterator {
			return baseIterator
		},
	}

	baseSeriesSet := &mockSeriesSet{
		atFunc: func() storage.Series {
			return baseSeries
		},
	}

	ss := upsampler.NewSeriesSet(baseSeriesSet, 60000, 60000, upsampler.CounterFunc)
	series := ss.At()

	// Create first iterator
	it1 := series.Iterator(nil)
	s.NotNil(it1)

	// Reuse same iterator instance
	it2 := series.Iterator(it1)

	// Should return the same iterator, just reset
	s.Equal(it1, it2)
}
