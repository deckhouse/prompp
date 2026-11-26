package storage

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// BenchmarkCompactingChunkSeriesMerger measures chunk iteration across overlapping in-memory blocks.
func BenchmarkCompactingChunkSeriesMerger(b *testing.B) {
	merge := NewCompactingChunkSeriesMerger(ChainedSeriesMerge)
	for _, tc := range []struct {
		name            string
		differentValues bool
	}{
		{name: "duplicates"},
		{name: "reencode", differentValues: true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			inputs := benchmarkOverlappingChunkInputs(b, tc.differentValues)
			var iter chunks.Iterator
			b.ReportAllocs()

			for b.Loop() {
				iter = merge(inputs...).Iterator(iter)
				for iter.Next() {
					_ = iter.At()
				}

				if err := iter.Err(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// benchmarkOverlappingChunkInputs builds three blocks with half of each block overlapping its neighbor.
func benchmarkOverlappingChunkInputs(b *testing.B, differentValues bool) []ChunkSeries {
	const (
		blockCount      = 3
		chunksPerBlock  = 4
		chunksPerStep   = chunksPerBlock / 2
		samplesPerChunk = 120
	)
	inputs := make([]ChunkSeries, blockCount)
	lset := labels.FromStrings("job", "overlap")
	samples := make([]chunks.Sample, samplesPerChunk)
	for blockIdx := range inputs {
		metas := make([]chunks.Meta, chunksPerBlock)
		for chunkIdx := range metas {
			mint := int64((blockIdx*chunksPerStep + chunkIdx) * samplesPerChunk)
			for i := range samples {
				ts := mint + int64(i)
				value := float64(ts)
				if differentValues {
					value += float64(blockIdx)
				}
				samples[i] = fSample{ts, value}
			}

			meta, err := chunks.ChunkFromSamples(samples)
			require.NoError(b, err)
			metas[chunkIdx] = meta
		}

		inputs[blockIdx] = &ChunkSeriesEntry{
			Lset: lset,
			ChunkIteratorFn: func(reuse chunks.Iterator) chunks.Iterator {
				if it, ok := reuse.(*listChunkSeriesIterator); ok {
					it.Reset(metas...)
					return it
				}

				return NewListChunkSeriesIterator(metas...)
			},
		}
	}

	return inputs
}
