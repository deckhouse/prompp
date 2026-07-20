package block

import (
	"fmt"
	"sync"
	"testing"

	"github.com/oklog/ulid"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/stretchr/testify/require"
)

func TestCompactorCompactUsesPlanAndSource(t *testing.T) {
	t.Parallel()

	wantUID := ulid.MustNew(1, nil)
	fake := &fakeCompactor{
		plan:   []string{"01AAA", "01BBB"},
		result: []ulid.ULID{wantUID},
	}
	source := &fakeBlockSource{
		blocks: []*tsdb.Block{nil, nil},
	}

	c := &Compactor{
		dir:       "/tmp/data",
		compactor: fake,
		source:    source,
		metrics:   newCompactorMetrics(nil),
	}

	uids, compacted, err := c.Compact()
	require.NoError(t, err)
	require.True(t, compacted)
	require.Equal(t, []ulid.ULID{wantUID}, uids)
	require.True(t, fake.compactCalled)
	require.Equal(t, "/tmp/data", fake.compactDest)
	require.Equal(t, []string{"01AAA", "01BBB"}, fake.compactDirs)
	require.Len(t, fake.compactOpen, 2)
}

func TestCompactorCompactNoPlanIsNoop(t *testing.T) {
	t.Parallel()

	fake := &fakeCompactor{plan: nil}
	c := &Compactor{
		dir:       "/tmp/data",
		compactor: fake,
		source:    &fakeBlockSource{},
		metrics:   newCompactorMetrics(nil),
	}

	uids, compacted, err := c.Compact()
	require.NoError(t, err)
	require.False(t, compacted)
	require.Nil(t, uids)
	require.Equal(t, 1, fake.planCalls)
	require.False(t, fake.compactCalled)
}

func TestCompactionRanges(t *testing.T) {
	t.Parallel()

	t.Run("without max duration", func(t *testing.T) {
		t.Parallel()
		ranges := compactionRanges(2*60*60*1000, 0)
		require.Equal(t, []int64{
			2 * 60 * 60 * 1000,
			6 * 60 * 60 * 1000,
			18 * 60 * 60 * 1000,
			54 * 60 * 60 * 1000,
			162 * 60 * 60 * 1000,
			486 * 60 * 60 * 1000,
			1458 * 60 * 60 * 1000,
			4374 * 60 * 60 * 1000,
			13122 * 60 * 60 * 1000,
			39366 * 60 * 60 * 1000,
		}, ranges)
	})

	t.Run("with max duration", func(t *testing.T) {
		t.Parallel()
		ranges := compactionRanges(2*60*60*1000, 31*24*60*60*1000)
		require.Equal(t, []int64{
			2 * 60 * 60 * 1000,
			6 * 60 * 60 * 1000,
			18 * 60 * 60 * 1000,
			54 * 60 * 60 * 1000,
			162 * 60 * 60 * 1000,
			486 * 60 * 60 * 1000,
		}, ranges)
	})

	t.Run("max lower than min is normalized", func(t *testing.T) {
		t.Parallel()
		ranges := compactionRanges(2*60*60*1000, 60*60*1000)
		require.Equal(t, []int64{2 * 60 * 60 * 1000}, ranges)
	})
}

type fakeBlockSource struct {
	blocks []*tsdb.Block
}

func (f *fakeBlockSource) Blocks() []*tsdb.Block {
	return f.blocks
}

type fakeCompactor struct {
	mu sync.Mutex

	plan   []string
	result []ulid.ULID

	planCalls int

	compactCalls  int
	compactCalled bool
	compactDest   string
	compactDirs   []string
	compactOpen   []*tsdb.Block
}

func (f *fakeCompactor) Plan(string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.planCalls++
	return append([]string(nil), f.plan...), nil
}

func (f *fakeCompactor) Write(string, tsdb.BlockReader, int64, int64, *tsdb.BlockMeta) ([]ulid.ULID, error) {
	return nil, fmt.Errorf("not implemented")
}

func (f *fakeCompactor) Compact(dest string, dirs []string, open []*tsdb.Block) ([]ulid.ULID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.compactCalls++
	f.compactCalled = true
	f.compactDest = dest
	f.compactDirs = append([]string(nil), dirs...)
	f.compactOpen = append([]*tsdb.Block(nil), open...)
	return append([]ulid.ULID(nil), f.result...), nil
}
