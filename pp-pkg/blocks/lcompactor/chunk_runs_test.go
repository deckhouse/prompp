package lcompactor

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/testutils"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// TestChunkCollectorFallsBackOnOverlap checks that a partial result is replaced, not appended to.
func TestChunkCollectorFallsBackOnOverlap(t *testing.T) {
	lset := labels.FromStrings("job", "test")
	first := storage.NewListChunkSeriesFromSamples(lset,
		[]chunks.Sample{testutils.SampleTest{TS: 1, V: 1}},
		[]chunks.Sample{testutils.SampleTest{TS: 2, V: 2}, testutils.SampleTest{TS: 3, V: 3}},
	)
	second := storage.NewListChunkSeriesFromSamples(lset,
		[]chunks.Sample{testutils.SampleTest{TS: 3, V: 3}, testutils.SampleTest{TS: 4, V: 4}},
	)
	empty := storage.NewListChunkSeriesFromSamples(lset)
	merge := storage.NewCompactingChunkSeriesMerger(storage.ChainedSeriesMerge)
	inputs := []storage.ChunkSeries{second, empty, first}

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
