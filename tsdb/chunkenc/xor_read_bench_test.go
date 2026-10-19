package chunkenc

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// BenchmarkXORReadPatterns measures complete decoding, including Seek prefixes, with iterator reuse.
func BenchmarkXORReadPatterns(b *testing.B) {
	for _, pattern := range []string{
		"constant", "counter", "gauge", "jitter_constant", "jitter_counter", "jitter_gauge", "wide_bits",
	} {
		b.Run(pattern, func(b *testing.B) {
			for _, size := range []int{120, 1024} {
				chunk, samples := xorReadFixture(b, pattern, size)

				b.Run(fmt.Sprintf("samples=%d", size), func(b *testing.B) {
					for _, seek := range []bool{false, true} {
						b.Run(fmt.Sprintf("seek=%t", seek), func(b *testing.B) {
							start := 0

							if seek {
								start = size / 2
							}

							var expected uint64

							for _, sample := range samples[start:] {
								expected ^= uint64(sample.t) ^ math.Float64bits(sample.v)
							}

							var iterator Iterator
							var checksum uint64
							var count int
							b.ReportAllocs()
							b.SetBytes(int64(len(chunk.Bytes())))
							b.ResetTimer()

							for b.Loop() {
								iterator = chunk.Iterator(iterator)
								checksum, count = 0, 0
								var valueType ValueType

								if seek {
									valueType = iterator.Seek(samples[start].t)
								} else {
									valueType = iterator.Next()
								}

								for valueType == ValFloat {
									ts, value := iterator.At()
									checksum ^= uint64(ts) ^ math.Float64bits(value)
									count++
									valueType = iterator.Next()
								}

								if err := iterator.Err(); err != nil {
									b.Fatal(err)
								}
							}

							b.StopTimer()
							require.Equal(b, size-start, count)
							require.Equal(b, expected, checksum)
							b.ReportMetric(float64(size), "decoded_samples/op")
						})
					}
				})
			}
		})
	}
}

func xorReadFixture(tb testing.TB, pattern string, size int) (*XORChunk, []pair) {
	tb.Helper()
	chunk := NewXORChunk()
	appender, err := chunk.Appender()
	require.NoError(tb, err)
	samples := make([]pair, size)
	randomBits := uint64(0x123456789abcdef)

	for i := range samples {
		ts := int64(i) * 10_000
		value := float64(i)

		switch pattern {
		case "constant":
			value = 42
		case "counter":
		case "gauge":
			value = math.Sin(float64(i) * 0.1)
		case "jitter_constant":
			ts += int64(i%7) * 37
			value = 42
		case "jitter_counter":
			ts += int64(i%7) * 37
		case "jitter_gauge":
			ts += int64(i%7) * 37
			value = math.Sin(float64(i) * 0.1)
		case "wide_bits":
			randomBits ^= randomBits << 13
			randomBits ^= randomBits >> 7
			randomBits ^= randomBits << 17
			value = math.Float64frombits(randomBits&0x800fffffffffffff | uint64(1023+i%2)<<52)
		default:
			tb.Fatalf("unknown XOR pattern %q", pattern)
		}

		samples[i] = pair{t: ts, v: value}
		appender.Append(ts, value)
	}

	return chunk, samples
}
