package lcompactor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
	"github.com/prometheus/prometheus/pp-pkg/blocks/testutils"
	"github.com/prometheus/prometheus/storage"
)

// BenchmarkCompactDisjointChunks compares the two merges on the same small on-disk inputs.
func BenchmarkCompactDisjointChunks(b *testing.B) {
	const (
		blockCount      = 3
		seriesCount     = 96
		labelCount      = 5
		samplesPerBlock = 480
	)
	inputs := b.TempDir()
	var dirs [blockCount]string
	var blocks [blockCount]*block.Block
	for i := range dirs {
		mint := int64(i * samplesPerBlock)
		dirs[i] = testutils.CreateBlock(b, inputs, testutils.GenSeries(seriesCount, labelCount, mint, mint+samplesPerBlock))
		opened, err := block.OpenBlock(nil, dirs[i], nil)
		require.NoError(b, err)
		b.Cleanup(func() { require.NoError(b, opened.Close()) })
		blocks[i] = opened
	}

	cases := []struct {
		name  string
		merge storage.VerticalChunkSeriesMergeFunc
	}{
		{name: "heap", merge: storage.NewCompactingChunkSeriesMerger(storage.ChainedSeriesMerge)},
		{name: "sequential"},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			compactor, err := NewLeveledCompactor(b.Context(), nil, nil, []int64{0}, nil, tc.merge)
			require.NoError(b, err)
			dest := b.TempDir()

			b.ReportAllocs()
			for b.Loop() {
				ids, err := compactor.Compact(dest, dirs[:], blocks[:])
				if err != nil {
					b.Fatal(err)
				}

				b.StopTimer()
				require.Len(b, ids, 1)
				require.NoError(b, os.RemoveAll(filepath.Join(dest, ids[0].String())))
				b.StartTimer()
			}
		})
	}
}
