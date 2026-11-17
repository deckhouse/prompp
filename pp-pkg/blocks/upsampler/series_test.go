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
	// Arrange
	c := chunkenc.NewXORChunk()
	appender, err := c.Appender()
	s.Require().NoError(err)
	appender.Append(100, 1)
	appender.Append(200, 2)

	var baseIterators []chunkenc.Iterator
	baseSeries := &storage.SeriesEntry{
		SampleIteratorFn: func(reuse chunkenc.Iterator) chunkenc.Iterator {
			actual := c.Iterator(reuse)
			baseIterators = append(baseIterators, actual)

			return actual
		},
	}
	baseSet := &mockSeriesSet{
		atFunc: func() storage.Series { return baseSeries },
	}
	series := upsampler.NewSeriesSet(baseSet, 100, 100, upsampler.CounterFunc).At()
	first := series.Iterator(nil)
	s.Require().Equal(chunkenc.ValFloat, first.Seek(150))
	s.Require().Equal(int64(150), first.AtT())

	// Act
	reused := series.Iterator(first)

	// Assert
	s.Require().Same(first, reused)
	s.Require().Len(baseIterators, 2)
	s.Require().Same(baseIterators[0], baseIterators[1])
	s.Require().Equal(chunkenc.ValFloat, reused.Next())
	t, v := reused.At()
	s.Require().Equal(int64(100), t)
	s.Require().Equal(float64(1), v)
}

// BenchmarkSeriesIteratorReuse measures wrapper and base iterator reuse over a real XOR chunk.
func BenchmarkSeriesIteratorReuse(b *testing.B) {
	c := chunkenc.NewXORChunk()
	appender, err := c.Appender()
	if err != nil {
		b.Fatal(err)
	}
	appender.Append(100, 1)
	appender.Append(200, 2)

	baseSeries := &storage.SeriesEntry{SampleIteratorFn: c.Iterator}
	baseSet := &mockSeriesSet{
		atFunc: func() storage.Series { return baseSeries },
	}
	series := upsampler.NewSeriesSet(baseSet, 100, 100, upsampler.CounterFunc).At()
	it := series.Iterator(nil)
	b.ReportAllocs()

	for b.Loop() {
		it = series.Iterator(it)
	}
}
