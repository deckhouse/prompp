package upsampler_test

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/pp-pkg/blocks/upsampler"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// StaleIteratorSuite verifies interpolation boundaries around stale markers in real chunks.
type StaleIteratorSuite struct {
	suite.Suite

	chunk chunkenc.Chunk
}

func TestStaleIteratorSuite(t *testing.T) {
	suite.Run(t, new(StaleIteratorSuite))
}

func (s *StaleIteratorSuite) SetupTest() {
	s.chunk = chunkenc.NewXORChunk()
	appender, err := s.chunk.Appender()
	s.Require().NoError(err)
	appender.Append(100, 10)
	appender.Append(200, math.Float64frombits(value.StaleNaN))
	appender.Append(300, 30)
	appender.Append(400, 50)
}

func (s *StaleIteratorSuite) TestNextPreservesStaleBoundaryAndResumesInterpolation() {
	stale := math.Float64frombits(value.StaleNaN)
	for _, tc := range []struct {
		kind   upsampler.FuncKind
		values []float64
	}{
		{kind: upsampler.GaugeFunc, values: []float64{10, stale, 30, 35, 40, 45, 50}},
		{kind: upsampler.CounterFunc, values: []float64{10, stale, 30, 35, 40, 45, 50}},
		{kind: upsampler.OverTimeFunc, values: []float64{10, stale, 30, 30, 30, 30, 50}},
	} {
		s.Run(strconv.FormatUint(uint64(tc.kind), 10), func() {
			// Arrange
			it := s.newStaleIterator(tc.kind)

			// Act
			for i, timestamp := range []int64{100, 200, 300, 325, 350, 375, 400} {
				valueType := it.Next()

				// Assert
				s.Require().Equal(chunkenc.ValFloat, valueType)
				t, v := it.At()
				s.Require().Equal(timestamp, t)
				s.Require().Equal(math.Float64bits(tc.values[i]), math.Float64bits(v))
			}

			s.Require().Equal(chunkenc.ValNone, it.Next())
			s.Require().NoError(it.Err())
		})
	}
}

func (s *StaleIteratorSuite) TestSeekPreservesStaleBoundary() {
	for _, kind := range []upsampler.FuncKind{upsampler.GaugeFunc, upsampler.CounterFunc, upsampler.OverTimeFunc} {
		for _, tc := range []struct {
			name   string
			start  int64
			target int64
			want   int64
			bits   uint64
		}{
			{name: "initial_before_marker", target: 150, want: 200, bits: value.StaleNaN},
			{name: "initialized_before_marker", start: 100, target: 150, want: 200, bits: value.StaleNaN},
			{name: "initial_after_marker", target: 250, want: 300, bits: math.Float64bits(30)},
			{name: "advance_past_marker", start: 100, target: 250, want: 300, bits: math.Float64bits(30)},
			{name: "from_marker", start: 200, target: 250, want: 300, bits: math.Float64bits(30)},
		} {
			s.Run(strconv.FormatUint(uint64(kind), 10)+"/"+tc.name, func() {
				// Arrange
				it := s.newStaleIterator(kind)
				if tc.start != 0 {
					s.Require().Equal(chunkenc.ValFloat, it.Seek(tc.start))
					s.Require().Equal(tc.start, it.AtT())
				}

				// Act
				valueType := it.Seek(tc.target)

				// Assert
				s.Require().Equal(chunkenc.ValFloat, valueType)
				t, v := it.At()
				s.Require().Equal(tc.want, t)
				s.Require().Equal(tc.bits, math.Float64bits(v))
			})
		}
	}
}

func (s *StaleIteratorSuite) newStaleIterator(kind upsampler.FuncKind) *upsampler.Iterator {
	return upsampler.NewIterator(s.chunk.Iterator(nil), 25, 200, kind)
}
