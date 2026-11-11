package upsampler_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/pp-pkg/blocks/upsampler"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// HistogramIteratorSuite checks typed passthrough with real chunk iterators.
type HistogramIteratorSuite struct {
	suite.Suite

	histograms [2]chunkenc.Chunk
	prefix     chunkenc.Chunk
	suffix     chunkenc.Chunk
}

func TestHistogramIteratorSuite(t *testing.T) {
	suite.Run(t, new(HistogramIteratorSuite))
}

func (s *HistogramIteratorSuite) SetupTest() {
	s.histograms = [2]chunkenc.Chunk{chunkenc.NewHistogramChunk(), chunkenc.NewFloatHistogramChunk()}
	for _, c := range s.histograms {
		appender, err := c.Appender()
		s.Require().NoError(err)

		for count := int64(1); count <= 3; count++ {
			h := &histogram.Histogram{
				Count: uint64(count), ZeroCount: uint64(count), ZeroThreshold: 1, Sum: float64(count) / 2,
			}

			var replacement chunkenc.Chunk
			if c.Encoding() == chunkenc.EncHistogram {
				replacement, _, appender, err = appender.AppendHistogram(nil, count*100, h, false)
			} else {
				replacement, _, appender, err = appender.AppendFloatHistogram(nil, count*100, h.ToFloat(nil), false)
			}
			s.Require().NoError(err)
			s.Require().Nil(replacement)
		}
	}

	s.prefix = chunkenc.NewXORChunk()
	appender, err := s.prefix.Appender()
	s.Require().NoError(err)
	appender.Append(50, 5)

	s.suffix = chunkenc.NewXORChunk()
	appender, err = s.suffix.Appender()
	s.Require().NoError(err)
	appender.Append(400, 40)
	appender.Append(500, 50)
}

func (s *HistogramIteratorSuite) TestNextPreservesHistograms() {
	for _, c := range s.histograms {
		s.Run(c.Encoding().String(), func() {
			// Arrange
			base := c.Iterator(nil)
			it := upsampler.NewIterator(c.Iterator(nil), 25, 200, upsampler.GaugeFunc)
			reuse := &histogram.FloatHistogram{}

			// Act
			for expectedType := base.Next(); expectedType != chunkenc.ValNone; expectedType = base.Next() {
				actualType := it.Next()

				// Assert
				s.Require().Equal(expectedType, actualType)
				s.Require().Equal(base.AtT(), it.AtT())
				expectedTime, expected := base.AtFloatHistogram(nil)
				actualTime, actual := it.AtFloatHistogram(reuse)
				s.Require().Equal(expectedTime, actualTime)
				s.Require().Equal(expected, actual)
				s.Require().Same(reuse, actual)

				if actualType == chunkenc.ValHistogram {
					histogramReuse := &histogram.Histogram{}
					expectedTime, expected := base.AtHistogram(nil)
					actualTime, actual := it.AtHistogram(histogramReuse)
					s.Require().Equal(expectedTime, actualTime)
					s.Require().Equal(expected, actual)
					s.Require().Same(histogramReuse, actual)
				}
			}

			s.Require().Equal(chunkenc.ValNone, it.Next())
			s.Require().NoError(it.Err())
		})
	}
}

func (s *HistogramIteratorSuite) TestNextResetsFloatAnchorsAcrossHistograms() {
	for _, c := range s.histograms {
		s.Run(c.Encoding().String(), func() {
			// Arrange
			it := s.mixedIterator(c)
			expectedType := c.Iterator(nil).Next()

			// Act
			for _, timestamp := range []int64{50, 100, 200, 300, 400, 425, 450, 475, 500} {
				valueType := it.Next()

				// Assert
				s.Require().Equal(timestamp, it.AtT())
				if timestamp >= 100 && timestamp <= 300 {
					s.Require().Equal(expectedType, valueType)
					t, h := it.AtFloatHistogram(nil)
					s.Require().Equal(timestamp, t)
					s.Require().NotNil(h)
				} else {
					s.Require().Equal(chunkenc.ValFloat, valueType)
					t, v := it.At()
					s.Require().Equal(timestamp, t)
					s.Require().Equal(float64(timestamp)/10, v)
				}
			}

			s.Require().Equal(chunkenc.ValNone, it.Next())
			s.Require().NoError(it.Err())
		})
	}
}

func (s *HistogramIteratorSuite) TestSeekPreservesHistograms() {
	for _, c := range s.histograms {
		for _, tc := range []struct {
			name    string
			advance int
			target  int64
			want    int64
		}{
			{name: "initial", target: 150, want: 200},
			{name: "current", advance: 2, target: 100, want: 100},
			{name: "buffered", advance: 1, target: 75, want: 100},
			{name: "advanced", advance: 1, target: 250, want: 300},
			{name: "exact", advance: 1, target: 200, want: 200},
		} {
			s.Run(c.Encoding().String()+"/"+tc.name, func() {
				// Arrange
				it := s.mixedIterator(c)
				expectedType := c.Iterator(nil).Next()
				for range tc.advance {
					s.Require().NotEqual(chunkenc.ValNone, it.Next())
				}

				// Act
				valueType := it.Seek(tc.target)

				// Assert
				s.Require().Equal(expectedType, valueType)
				s.Require().Equal(tc.want, it.AtT())
				t, h := it.AtFloatHistogram(nil)
				s.Require().Equal(tc.want, t)
				s.Require().NotNil(h)
				s.Require().Equal(float64(tc.want)/100, h.Count)
			})
		}
	}
}

func (s *HistogramIteratorSuite) TestSeekResetsFloatAnchorsAcrossHistograms() {
	for _, c := range s.histograms {
		s.Run(c.Encoding().String(), func() {
			// Arrange
			it := s.mixedIterator(c)
			s.Require().Equal(chunkenc.ValFloat, it.Next())

			// Act
			valueType := it.Seek(350)

			// Assert
			s.Require().Equal(chunkenc.ValFloat, valueType)
			t, v := it.At()
			s.Require().Equal(int64(400), t)
			s.Require().Equal(float64(40), v)
			s.Require().Equal(chunkenc.ValFloat, it.Next())
			t, v = it.At()
			s.Require().Equal(int64(425), t)
			s.Require().Equal(42.5, v)
		})
	}
}

func (s *HistogramIteratorSuite) mixedIterator(c chunkenc.Chunk) *upsampler.Iterator {
	series := storage.ChainedSeriesMerge(
		&storage.SeriesEntry{SampleIteratorFn: s.prefix.Iterator},
		&storage.SeriesEntry{SampleIteratorFn: c.Iterator},
		&storage.SeriesEntry{SampleIteratorFn: s.suffix.Iterator},
	)
	return upsampler.NewIterator(series.Iterator(nil), 25, 200, upsampler.GaugeFunc)
}
