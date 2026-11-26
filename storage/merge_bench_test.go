package storage

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// BenchmarkCompactingChunkSeriesMerger measures chunk iteration with and without single-input rows.
func BenchmarkCompactingChunkSeriesMerger(b *testing.B) {
	merge := NewCompactingChunkSeriesMerger(ChainedSeriesMerge)
	for _, tc := range []struct {
		name            string
		differentValues bool
		alternate       bool
		sparse          bool
	}{
		{name: "duplicates"},
		{name: "reencode", differentValues: true},
		{name: "mixed_duplicates", alternate: true},
		{name: "mixed_reencode", differentValues: true, alternate: true},
		{name: "sparse_duplicates", sparse: true},
		{name: "sparse_reencode", differentValues: true, sparse: true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			inputs := benchmarkOverlappingChunkInputs(b, tc.differentValues, tc.sparse)
			var reuse ChunkIteratorReuse
			singleNext := false
			b.ReportAllocs()

			for b.Loop() {
				var series ChunkSeries
				if singleNext {
					// A series from one block bypasses the merger and replaces its iterator.
					series = inputs[0]
				} else {
					series = merge(inputs...)
				}
				singleNext = tc.alternate && !singleNext
				iter := reuse.Iterator(series)
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

// benchmarkOverlappingChunkInputs builds three blocks with either half a block or one chunk of overlap.
func benchmarkOverlappingChunkInputs(b *testing.B, differentValues, sparse bool) []ChunkSeries {
	const (
		blockCount           = 3
		denseChunksPerBlock  = 4
		sparseChunksPerBlock = 32
		samplesPerChunk      = 120
	)
	chunksPerBlock := denseChunksPerBlock
	chunksPerStep := chunksPerBlock / 2
	if sparse {
		chunksPerBlock = sparseChunksPerBlock
		chunksPerStep = chunksPerBlock - 1
	}

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
