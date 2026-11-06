package querier

import (
	"runtime"
	"strconv"
	"testing"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

func BenchmarkChunkSeriesChunksIterator(b *testing.B) {
	for _, count := range []int{1, 2, 16, 128} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			recoded := make([]cppbridge.RecodedChunk, count)

			for i := range recoded {
				samples := make([]chunkSeriesTestSample, 120)

				for j := range samples {
					samples[j] = chunkSeriesTestSample{timestamp: int64(i*120 + j), value: float64(i + j)}
				}

				recoded[i] = makeChunkSeriesTestChunk(b, samples)
			}

			b.Run("new", func(b *testing.B) {
				b.ReportAllocs()

				for b.Loop() {
					iterator := NewChunkSeriesChunksIterator(recoded)

					for iterator.Next() {
						meta := iterator.At()
						runtime.KeepAlive(meta.Chunk.Bytes())
					}

					runtime.KeepAlive(iterator)
				}
			})

			b.Run("reuse", func(b *testing.B) {
				iterator := NewChunkSeriesChunksIterator(recoded)
				b.ReportAllocs()

				for b.Loop() {
					iterator.ResetTo(recoded)

					for iterator.Next() {
						meta := iterator.At()
						runtime.KeepAlive(meta.Chunk.Bytes())
					}
				}
			})
		})
	}
}
