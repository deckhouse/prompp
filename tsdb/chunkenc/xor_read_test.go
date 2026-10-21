package chunkenc

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/value"
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
		"constant", "counter", "gauge", "jitter_constant", "jitter_counter", "jitter_gauge", "wide_bits",
	} {
		s.Run(pattern, func() {
			for _, size := range []int{0, 1, 2, 3, 7, 120, 1024} {
				chunk, samples := xorReadFixture(s.T(), pattern, size)
				s.assertIterator(chunk.Iterator(nil), samples)
			}
		})
	}
}

func (s *XORReadSuite) TestReadValueBufferBoundaries() {
	for _, width := range []uint8{1, 62, 63, 64} {
		for _, reuse := range []bool{false, true} {
			for _, valid := range []uint8{1, 2, 3, 63, 64} {
				s.Run(fmt.Sprintf("width=%d/reuse=%t/valid=%d", width, reuse, valid), func() {
					leading := min(uint8(31), 64-width)
					trailing := 64 - width - leading
					payload := uint64(1)<<(width-1) | 1
					expected := math.Float64frombits(math.Float64bits(42) ^ payload<<trailing)
					iterator := s.valueReader(valid, expected, leading, trailing, reuse)

					s.Require().Equal(ValFloat, iterator.readValue())
					s.Equal(leading, iterator.leading)
					s.Equal(trailing, iterator.trailing)

					if reuse && uint16(valid) >= 2+uint16(width) {
						s.Equal(valid-2-width, iterator.br.valid)
					}

					s.assertCurrent(iterator, pair{t: 10_000, v: expected})
					s.assertIterator(iterator, []pair{{t: 20_001, v: expected}})
				})
			}
		}
	}
}

func (s *XORReadSuite) TestReadValueRequiredBits() {
	for _, tc := range []struct {
		name string
		bits string
	}{
		{name: "first_control"},
		{name: "second_control", bits: "1"},
		{name: "new_window_leading", bits: "11"},
		{name: "new_window_width", bits: "1100000"},
		{name: "new_window_payload", bits: "1100000000001"},
		{name: "reuse_payload", bits: "10"},
		{name: "reuse_payload_63_bits", bits: "10" + strings.Repeat("1", 61)},
	} {
		s.Run(tc.name, func() {
			var buffer uint64

			if tc.bits != "" {
				var err error
				buffer, err = strconv.ParseUint(tc.bits, 2, 64)
				s.Require().NoError(err)
			}

			iterator := &xorIterator{
				br:      bstreamReader{buffer: buffer, valid: uint8(len(tc.bits))},
				numRead: 1, numTotal: 2,
			}
			s.Require().Equal(ValNone, iterator.readValue())
			s.Require().ErrorIs(iterator.Err(), io.EOF)
			s.Equal(uint16(1), iterator.numRead)
			s.Equal(ValNone, iterator.Next())
			s.Equal(ValNone, iterator.Seek(math.MaxInt64))

			iterator.Reset(s.chunk.Bytes())
			s.assertIterator(iterator, s.samples)
		})
	}
}

func (s *XORReadSuite) TestReadValueUnchangedPreservesTimestamp() {
	for _, valid := range []uint8{1, 2, 3, 63, 64} {
		s.Run(fmt.Sprintf("valid=%d", valid), func() {
			iterator := s.valueReader(valid, 42, 0, 0, true)
			s.Require().Equal(ValFloat, iterator.readValue())
			s.Equal(valid-1, iterator.br.valid)
			s.assertCurrent(iterator, pair{t: 10_000, v: 42})
			s.assertIterator(iterator, []pair{{t: 20_001, v: 42}})
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

func (s *XORReadSuite) TestSeekWithTimestampJitter() {
	chunk, samples := xorReadFixture(s.T(), "jitter_constant", 1024)
	iterator := chunk.Iterator(nil)

	for _, position := range []int{2, 3, 6, 7, 8, 63, 64, 511, 1023} {
		s.Require().Equal(ValFloat, iterator.Seek(samples[position].t))
		s.assertCurrent(iterator, samples[position])
	}

	s.assertIterator(iterator, nil)
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

func (s *XORReadSuite) valueReader(valid uint8, next float64, leading, trailing uint8, reuse bool) *xorIterator {
	s.T().Helper()
	stream := &bstream{}
	stream.writeBits(0, int(64-valid))

	iterator := &xorIterator{
		val: 42, t: 10_000, tDelta: 10_000, numRead: 1, numTotal: 3,
	}

	if reuse {
		iterator.leading, iterator.trailing = leading, trailing
	} else {
		leading, trailing = 0xff, 0
	}

	xorWrite(stream, next, 42, &leading, &trailing)
	stream.writeBits(0b10, 2)
	stream.writeBits(1, 14)
	xorWrite(stream, next, next, &leading, &trailing)
	stream.stream = append(stream.stream, make([]byte, 16)...)
	iterator.br = newBReader(stream.bytes())
	s.Require().True(iterator.br.loadNextBuffer(1))
	_, err := iterator.br.readBits(64 - valid)
	s.Require().NoError(err)
	s.Require().Equal(valid, iterator.br.valid)

	return iterator
}

func FuzzXORReadRoundTrip(f *testing.F) {
	for _, seed := range [][]byte{
		{0},
		{1, 2, 3, 4, 5, 6, 7, 8, 9},
		{2, 255, 0, 17},
		{3, 0, 0, 0, 0, 0, 0, 0x80},
		{3, 1, 0, 0, 0, 0, 0xf0, 0x7f},
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 || len(data) > 2048 {
			t.Skip()
		}

		chunk := NewXORChunk()
		appender, err := chunk.Appender()
		require.NoError(t, err)
		samples := []pair{{t: -1_000_000_000, v: math.Float64frombits(value.StaleNaN)}}
		appender.Append(samples[0].t, samples[0].v)

		for i := 0; i < len(data); i += 8 {
			var encoded [8]byte
			copy(encoded[:], data[i:])
			next := math.Float64frombits(binary.LittleEndian.Uint64(encoded[:]))

			switch data[0] % 4 {
			case 0:
				next = 42
			case 1:
				next = float64(i / 32)
			case 2:
				next = math.Sin(float64(i))
			}

			sample := pair{t: samples[len(samples)-1].t + 1 + int64(data[i]), v: next}
			samples = append(samples, sample)
			appender.Append(sample.t, sample.v)
		}

		iterator := chunk.Iterator(nil)

		for _, sample := range samples {
			require.Equal(t, ValFloat, iterator.Next())
			ts, actual := iterator.At()
			require.Equal(t, sample.t, ts)
			require.Equal(t, math.Float64bits(sample.v), math.Float64bits(actual))
		}

		require.Equal(t, ValNone, iterator.Next())
		require.NoError(t, iterator.Err())
		iterator = chunk.Iterator(iterator)

		for _, sample := range samples {
			require.Equal(t, ValFloat, iterator.Seek(sample.t))
			ts, actual := iterator.At()
			require.Equal(t, sample.t, ts)
			require.Equal(t, math.Float64bits(sample.v), math.Float64bits(actual))
		}

		require.Equal(t, ValNone, iterator.Next())
		require.NoError(t, iterator.Err())
		length := 2 + int(data[0])%(len(chunk.Bytes())-2)
		truncated, err := FromData(EncXOR, chunk.Bytes()[:length])
		require.NoError(t, err)
		iterator = truncated.Iterator(iterator)
		count := 0

		for iterator.Next() == ValFloat {
			count++
			require.LessOrEqual(t, count, len(samples))
		}
	})
}
