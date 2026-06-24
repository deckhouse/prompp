package block

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunks"
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

func createTestBlock(t *testing.T, dir string, startTS int, metric string) {
	t.Helper()

	series := []storage.Series{
		storage.NewListSeries(labels.FromStrings("__name__", metric), chunks.GenerateSamples(startTS, 2)),
	}
	_, err := tsdb.CreateBlock(series, dir, 0, log.NewNopLogger())
	require.NoError(t, err)
}
