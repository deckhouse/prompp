package lcompactor

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/testutils"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// TestChunkCollectorFallsBackOnOverlap checks that a retry preserves the standard merge result.
func TestChunkCollectorFallsBackOnOverlap(t *testing.T) {
	lset := labels.FromStrings("job", "test")
	prefix := storage.NewListChunkSeriesFromSamples(lset,
		[]chunks.Sample{testutils.SampleTest{TS: 1, V: 1}},
	)
	first := storage.NewListChunkSeriesFromSamples(lset,
		[]chunks.Sample{testutils.SampleTest{TS: 3, V: 3}, testutils.SampleTest{TS: 4, V: 4}},
	)
	second := storage.NewListChunkSeriesFromSamples(lset,
		[]chunks.Sample{testutils.SampleTest{TS: 3, V: 30}, testutils.SampleTest{TS: 4, V: 4}},
	)
	empty := storage.NewListChunkSeriesFromSamples(lset)
	merge := storage.NewCompactingChunkSeriesMerger(storage.ChainedSeriesMerge)
	// Equal chunk bounds and different values make reordering visible in the merged samples.
	inputs := []storage.ChunkSeries{second, empty, first, prefix}

	var want, got chunkCollector
	require.NoError(t, want.collect(merge(inputs...)))
	require.NoError(t, got.collect(disjointChunkSeriesMerger(merge)(inputs...)))

	require.Len(t, got.metas, 2)
	for i, chunk := range got.metas {
		require.Equal(t, want.metas[i].MinTime, chunk.MinTime)
		require.Equal(t, want.metas[i].MaxTime, chunk.MaxTime)
		require.Equal(t, want.metas[i].Chunk.Bytes(), chunk.Chunk.Bytes())
	}
}

// TestChunkCollectorSkipsFallbackForDisjointChunks checks that disjoint chunks are collected without a heap.
func TestChunkCollectorSkipsFallbackForDisjointChunks(t *testing.T) {
	lset := labels.FromStrings("job", "test")
	first := storage.NewListChunkSeriesFromSamples(lset,
		[]chunks.Sample{testutils.SampleTest{TS: 1, V: 1}, testutils.SampleTest{TS: 2, V: 2}},
		[]chunks.Sample{testutils.SampleTest{TS: 3, V: 3}},
	)
	second := storage.NewListChunkSeriesFromSamples(lset,
		[]chunks.Sample{testutils.SampleTest{TS: 4, V: 4}, testutils.SampleTest{TS: 5, V: 5}},
	)
	empty := storage.NewListChunkSeriesFromSamples(lset)
	merge := storage.NewCompactingChunkSeriesMerger(storage.ChainedSeriesMerge)
	fallbackCalls := 0
	fallback := func(inputs ...storage.ChunkSeries) storage.ChunkSeries {
		fallbackCalls++
		return merge(inputs...)
	}

	var collector chunkCollector
	require.NoError(t, collector.collect(disjointChunkSeriesMerger(fallback)(second, empty, first)))

	require.Zero(t, fallbackCalls, "disjoint chunks must not use the fallback merger")
	want := [][2]int64{{1, 2}, {3, 3}, {4, 5}}
	require.Len(t, collector.metas, len(want))
	for i, chunk := range collector.metas {
		require.Equal(t, want[i], [2]int64{chunk.MinTime, chunk.MaxTime})
	}
}
