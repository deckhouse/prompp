package querier_test

import (
	"testing"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
)

func TestXxx(t *testing.T) {
	lss := &shard.LSS{}
	ds := shard.NewDataStorage()
	wl := &testWal{}
	sd := shard.NewShard(lss, ds, wl, 0)

	h := head.NewHead([]*shard.Shard[*testWal]{sd})

	querier.NewQuerier(
		h,
		querier.NewNoOpShardedDeduplicator,
		0,
		0,
		nil,
		querier.NewMetrics(nil, "test"),
	)
}

// testWal test implementation wal.
type testWal struct{}

// Commit test implementation wal.
func (*testWal) Commit() error {
	return nil
}

// Flush test implementation wal.
func (*testWal) Flush() error {
	return nil
}

// Write test implementation wal.
func (*testWal) Write(_ []*cppbridge.InnerSeries) (bool, error) {
	return false, nil
}
