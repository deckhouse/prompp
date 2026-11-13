package querier

import (
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/tsdb/chunks"
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

			series := newChunkSeriesTestSeries(recoded)

			b.Run("new", func(b *testing.B) {
				b.ReportAllocs()

				for b.Loop() {
					iterator := series.Iterator(nil)

					for iterator.Next() {
						meta := iterator.At()
						runtime.KeepAlive(meta.Chunk.Bytes())
					}

					runtime.KeepAlive(iterator)
				}
			})

			b.Run("reuse", func(b *testing.B) {
				iterator := series.Iterator(nil)
				b.ReportAllocs()

				for b.Loop() {
					iterator = series.Iterator(iterator)

					for iterator.Next() {
						meta := iterator.At()
						runtime.KeepAlive(meta.Chunk.Bytes())
					}
				}
			})
		})
	}
}

func BenchmarkChunkSeriesLifecycle(b *testing.B) {
	for _, count := range []int{1, 2, 16, 128} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			lss := cppbridge.NewQueryableLssStorage()
			lss.FindOrEmplace(model.NewLabelSetBuilder().Set(labels.MetricName, "chunk_series_benchmark").Build())
			dataStorage := cppbridge.NewDataStorage(false, false)
			sampleCount := count * cppbridge.MaxPointsInChunk

			for i := range sampleCount {
				dataStorage.Encode(0, int64(i)*15_000, float64(i%17))
			}

			snapshot := lss.CreateLabelSetSnapshot()
			interval := cppbridge.TimeInterval{MinT: 0, MaxT: int64(sampleCount) * 15_000}
			query := dataStorage.Query(cppbridge.DataStorageQuery{
				StartTimestampMs: interval.MinT,
				EndTimestampMs:   interval.MaxT,
				LabelSetIDs:      []uint32{0},
			})
			require.Equal(b, cppbridge.DataStorageQueryStatusSuccess, query.Status)
			newSet := func() *ChunkSeriesSet {
				recoder := cppbridge.NewSerializedChunkRecoder(query.SerializedData, interval)

				return NewChunkSeriesSet(snapshot, recoder)
			}
			set := newSet()
			require.True(b, set.Next())
			series := set.At()
			iterator := series.Iterator(nil)
			actualCount := 0

			for iterator.Next() {
				actualCount++
			}

			require.NoError(b, iterator.Err())
			require.Equal(b, count, actualCount)

			b.Run("build", func(b *testing.B) {
				var reused chunks.Iterator
				b.ReportAllocs()

				for b.Loop() {
					result := newSet()

					for result.Next() {
						reused = result.At().Iterator(reused)

						for reused.Next() {
							meta := reused.At()
							runtime.KeepAlive(meta.Chunk.Bytes())
						}
					}

					if err := result.Err(); err != nil {
						b.Fatal(err)
					}
				}
			})

			b.Run("iterate", func(b *testing.B) {
				var reused chunks.Iterator
				b.ReportAllocs()

				for b.Loop() {
					reused = series.Iterator(reused)

					for reused.Next() {
						meta := reused.At()
						runtime.KeepAlive(meta.Chunk.Bytes())
					}
				}
			})
		})
	}
}
