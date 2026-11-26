package lcompactor

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/testutils"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// BenchmarkChunkCollector compares the two collectors on pre-encoded, in-memory chunks.
func BenchmarkChunkCollector(b *testing.B) {
	heapMerge := storage.NewCompactingChunkSeriesMerger(storage.ChainedSeriesMerge)
	for _, chunksPerBlock := range []int{1, 4} {
		b.Run("chunks_per_block="+strconv.Itoa(chunksPerBlock), func(b *testing.B) {
			inputs := benchmarkChunkInputs(b, chunksPerBlock)
			for _, tc := range []struct {
				name  string
				merge storage.VerticalChunkSeriesMergeFunc
			}{
				{name: "heap", merge: heapMerge},
				{name: "sequential", merge: disjointChunkSeriesMerger(heapMerge)},
			} {
				b.Run("variant="+tc.name, func(b *testing.B) {
					var collector chunkCollector
					require.NoError(b, collector.collect(tc.merge(inputs...)))
					require.Len(b, collector.metas, len(inputs)*chunksPerBlock)
					b.ReportAllocs()

					for b.Loop() {
						if err := collector.collect(tc.merge(inputs...)); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

// benchmarkChunkInputs returns disjoint blocks in reverse time order, simulating an unordered series heap result.
func benchmarkChunkInputs(b *testing.B, chunksPerBlock int) []storage.ChunkSeries {
	const blockCount = 3
	lset := labels.FromStrings("job", "collector-bench")
	inputs := make([]storage.ChunkSeries, blockCount)
	for blockIdx := range inputs {
		metas := make([]chunks.Meta, chunksPerBlock)
		for chunkIdx := range metas {
			mint := int64(2 * (blockIdx*chunksPerBlock + chunkIdx))
			meta, err := chunks.ChunkFromSamples([]chunks.Sample{
				testutils.SampleTest{TS: mint, V: 1},
				testutils.SampleTest{TS: mint + 1, V: 2},
			})
			require.NoError(b, err)
			metas[chunkIdx] = meta
		}

		inputs[len(inputs)-1-blockIdx] = &storage.ChunkSeriesEntry{
			Lset: lset,
			ChunkIteratorFn: func(reuse chunks.Iterator) chunks.Iterator {
				if it, ok := reuse.(interface{ Reset(...chunks.Meta) }); ok {
					it.Reset(metas...)
					return reuse
				}

				return storage.NewListChunkSeriesIterator(metas...)
			},
		}
	}

	return inputs
}
