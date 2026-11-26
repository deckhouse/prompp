package storage

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// benchmarkOverlapLayout describes how the input chunks overlap.
type benchmarkOverlapLayout uint8

const (
	// benchmarkDenseOverlap overlaps adjacent inputs by half a block.
	benchmarkDenseOverlap benchmarkOverlapLayout = iota
	// benchmarkSparseOverlap overlaps adjacent inputs by one chunk.
	benchmarkSparseOverlap
	// benchmarkFourActive interleaves four inputs before a shared final chunk.
	benchmarkFourActive
)

// BenchmarkCompactingChunkSeriesMerger measures chunk iteration with and without single-input rows.
func BenchmarkCompactingChunkSeriesMerger(b *testing.B) {
	merge := NewCompactingChunkSeriesMerger(ChainedSeriesMerge)
	for _, tc := range []struct {
		name            string
		differentValues bool
		alternate       bool
		layout          benchmarkOverlapLayout
	}{
		{name: "duplicates", layout: benchmarkDenseOverlap},
		{name: "reencode", differentValues: true, layout: benchmarkDenseOverlap},
		{name: "mixed_duplicates", alternate: true, layout: benchmarkDenseOverlap},
		{name: "mixed_reencode", differentValues: true, alternate: true, layout: benchmarkDenseOverlap},
		{name: "sparse_duplicates", layout: benchmarkSparseOverlap},
		{name: "sparse_reencode", differentValues: true, layout: benchmarkSparseOverlap},
		{name: "four_active_duplicates", layout: benchmarkFourActive},
		{name: "four_active_reencode", differentValues: true, layout: benchmarkFourActive},
	} {
		b.Run(tc.name, func(b *testing.B) {
			inputs := benchmarkOverlappingChunkInputs(b, tc.differentValues, tc.layout)
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

// benchmarkOverlappingChunkInputs builds input chunks with the requested overlap layout.
func benchmarkOverlappingChunkInputs(b *testing.B, differentValues bool, layout benchmarkOverlapLayout) []ChunkSeries {
	const (
		defaultBlockCount    = 3
		fourActiveBlockCount = 4
		denseChunksPerBlock  = 4
		sparseChunksPerBlock = 32
		samplesPerChunk      = 120
	)
	blockCount := defaultBlockCount
	chunksPerBlock := denseChunksPerBlock
	chunksPerStep := chunksPerBlock / 2
	switch layout {
	case benchmarkSparseOverlap:
		chunksPerBlock = sparseChunksPerBlock
		chunksPerStep = chunksPerBlock - 1
	case benchmarkFourActive:
		blockCount = fourActiveBlockCount
		chunksPerBlock = sparseChunksPerBlock
	}

	inputs := make([]ChunkSeries, blockCount)
	lset := labels.FromStrings("job", "overlap")
	samples := make([]chunks.Sample, samplesPerChunk)
	for blockIdx := range inputs {
		metas := make([]chunks.Meta, chunksPerBlock)
		for chunkIdx := range metas {
			position := blockIdx*chunksPerStep + chunkIdx
			if layout == benchmarkFourActive {
				position = chunkIdx * blockCount
				// No input runs out before the final overlap, so every earlier chunk has three peers.
				if chunkIdx < chunksPerBlock-1 {
					position += blockIdx
				}
			}
			mint := int64(position * samplesPerChunk)
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
