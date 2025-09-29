package head_test

import (
	"testing"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
)

func TestXxx(t *testing.T) {
	lss := &shard.LSS{}
	ds := shard.NewDataStorage()
	wl := &testWal{}
	sd := shard.NewShard(lss, ds, nil, nil, wl, 0)
	id := "test-head-id"
	generation := uint64(0)

	h := head.NewHead(
		id,
		[]*shard.Shard[*testWal]{sd},
		shard.NewPerGoroutineShard[*testWal],
		nil,
		generation,
		nil,
	)

	t.Log(h)
}

// testWal test implementation wal.
type testWal struct{}

// Close test implementation wal.
func (*testWal) Close() error {
	return nil
}

// Commit test implementation wal.
func (*testWal) Commit() error {
	return nil
}

// CurrentSize test implementation wal.
func (*testWal) CurrentSize() int64 {
	return 0
}

// Flush test implementation wal.
func (*testWal) Flush() error {
	return nil
}

// Sync test implementation wal.
func (*testWal) Sync() error {
	return nil
}

// Write test implementation wal.
func (*testWal) Write(_ []*cppbridge.InnerSeries) (bool, error) {
	return false, nil
}
