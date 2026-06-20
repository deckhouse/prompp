package block

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/stretchr/testify/require"
)

func TestManagerLoadsExistingBlocksOnStartup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createTestBlock(t, dir, 1000, "metric_a")
	createTestBlock(t, dir, 5000, "metric_b")

	m, err := NewManager(dir, nil, nil, log.NewNopLogger(), nil)
	require.NoError(t, err)
	t.Cleanup(m.Close)

	blocks := m.Blocks()
	require.Len(t, blocks, 2)
	require.LessOrEqual(t, blocks[0].Meta().MinTime, blocks[1].Meta().MinTime)
}

func TestManagerAppliesBlocksToDeleteOnInitialReload(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createTestBlock(t, dir, 1000, "metric_a")
	createTestBlock(t, dir, 5000, "metric_b")

	var marked ulid.ULID
	blocksToDelete := func(blocks []*tsdb.Block) map[ulid.ULID]struct{} {
		if len(blocks) == 0 {
			return nil
		}
		if marked.Compare(ulid.ULID{}) == 0 {
			marked = blocks[0].Meta().ULID
		}
		return map[ulid.ULID]struct{}{marked: {}}
	}

	m, err := NewManager(dir, nil, blocksToDelete, log.NewNopLogger(), nil)
	require.NoError(t, err)
	t.Cleanup(m.Close)

	blocks := m.Blocks()
	require.Len(t, blocks, 1)
	require.NotEqual(t, marked, blocks[0].Meta().ULID)

	_, err = os.Stat(filepath.Join(dir, marked.String()))
	require.True(t, os.IsNotExist(err), "expected deleted block dir to be removed")
}

func TestManagerReturnsErrorOnInitialReloadFailure(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	notDir := filepath.Join(tmp, "not-a-directory")
	require.NoError(t, os.WriteFile(notDir, []byte("x"), 0o600))

	m, err := NewManager(notDir, nil, nil, log.NewNopLogger(), nil)
	require.Error(t, err)
	require.Nil(t, m)
}

func TestManagerExportsLoadedBlocksMetrics(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createTestBlock(t, dir, 1000, "metric_a")
	createTestBlock(t, dir, 5000, "metric_b")

	reg := prometheus.NewRegistry()
	m, err := NewManager(dir, nil, nil, log.NewNopLogger(), reg)
	require.NoError(t, err)
	t.Cleanup(m.Close)

	require.Equal(t, float64(2), testutil.ToFloat64(m.metrics.loadedBlocks))
	require.Greater(t, testutil.ToFloat64(m.metrics.symbolTableSize), 0.0)

	durationCounts := map[int64]int{}
	for _, b := range m.Blocks() {
		duration := b.Meta().MaxTime - b.Meta().MinTime
		durationCounts[duration]++
	}
	for duration, count := range durationCounts {
		require.Equal(
			t,
			float64(count),
			testutil.ToFloat64(m.metrics.loadedBlocksByDuration.WithLabelValues(strconv.FormatInt(duration, 10))),
		)
	}
}

func createTestBlock(t *testing.T, dir string, startTS int, metric string) {
	t.Helper()

	series := []storage.Series{
		storage.NewListSeries(labels.FromStrings("__name__", metric), chunks.GenerateSamples(startTS, 2)),
	}
	_, err := tsdb.CreateBlock(series, dir, 0, log.NewNopLogger())
	require.NoError(t, err)
}
