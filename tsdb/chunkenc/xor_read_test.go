package chunkenc

import (
	"math"
	"testing"

	"github.com/stretchr/testify/suite"
)

// XORReadSuite checks decoding and iterator lifecycle against the exact input float bits.
type XORReadSuite struct {
	suite.Suite

	chunk   *XORChunk
	samples []pair
}

func TestXORReadSuite(t *testing.T) {
	suite.Run(t, new(XORReadSuite))
}

func (s *XORReadSuite) SetupTest() {
	s.chunk, s.samples = xorReadFixture(s.T(), "counter", 1024)
}

func (s *XORReadSuite) TestAppenderSnapshot() {
	iterator := s.chunk.Iterator(nil)
	appender, err := s.chunk.Appender()
	s.Require().NoError(err)
	last := s.samples[len(s.samples)-1]
	appender.Append(last.t+10_000, last.v+1)
	s.assertIterator(iterator, s.samples)

	iterator = s.chunk.Iterator(iterator)
	s.assertIterator(iterator, append(s.samples, pair{t: last.t + 10_000, v: last.v + 1}))
}

func (s *XORReadSuite) TestPatterns() {
	for _, pattern := range []string{
		"constant", "counter", "gauge", "jitter_counter", "jitter_gauge", "wide_bits",
	} {
		s.Run(pattern, func() {
			for _, size := range []int{0, 1, 2, 3, 7, 120, 1024} {
				chunk, samples := xorReadFixture(s.T(), pattern, size)
				s.assertIterator(chunk.Iterator(nil), samples)
			}
		})
	}
}

func (s *XORReadSuite) TestSeekAndReuse() {
	var iterator Iterator

	for _, position := range []int{0, 1, 63, 64, 511, 1023} {
		iterator = s.chunk.Iterator(iterator)
		s.Require().Equal(ValFloat, iterator.Seek(s.samples[position].t))
		s.assertCurrent(iterator, s.samples[position])
		s.Require().Equal(ValFloat, iterator.Seek(s.samples[position].t))
		s.assertCurrent(iterator, s.samples[position])
		s.assertIterator(iterator, s.samples[position+1:])
	}
}

func (s *XORReadSuite) TestSpecialValues() {
	bits := []uint64{
		0, 1 << 63, math.Float64bits(1), math.Float64bits(-1),
		math.Float64bits(math.MaxFloat64), math.Float64bits(math.SmallestNonzeroFloat64),
		math.Float64bits(math.Inf(1)), math.Float64bits(math.Inf(-1)),
		0x7ff8000000000001, 0x7ff0000000000002,
	}
	chunk := NewXORChunk()
	appender, err := chunk.Appender()
	s.Require().NoError(err)
	samples := make([]pair, 1024)

	for i := range samples {
		samples[i] = pair{t: int64(i) * 10_000, v: math.Float64frombits(bits[i%len(bits)])}
		appender.Append(samples[i].t, samples[i].v)
	}

	s.assertIterator(chunk.Iterator(nil), samples)
}

func (s *XORReadSuite) TestTruncatedChunk() {
	data := s.chunk.Bytes()

	for _, length := range []int{2, 3, 7, len(data) / 2, len(data) - 8} {
		chunk, err := FromData(EncXOR, data[:length])
		s.Require().NoError(err)
		iterator := chunk.Iterator(nil)
		count := 0

		for iterator.Next() == ValFloat {
			count++
		}

		s.Require().Error(iterator.Err())
		s.Less(count, len(s.samples))
		s.Equal(ValNone, iterator.Next())
		s.Equal(ValNone, iterator.Seek(math.MaxInt64))
	}
}

func (s *XORReadSuite) assertCurrent(iterator Iterator, expected pair) {
	ts, value := iterator.At()
	s.Require().Equal(expected.t, ts)
	s.Require().Equal(math.Float64bits(expected.v), math.Float64bits(value))
}

func (s *XORReadSuite) assertIterator(iterator Iterator, expected []pair) {
	for _, sample := range expected {
		s.Require().Equal(ValFloat, iterator.Next())
		s.assertCurrent(iterator, sample)
	}

	s.Require().Equal(ValNone, iterator.Next())
	s.Require().NoError(iterator.Err())
}
